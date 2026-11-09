package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"ops-go/pkg/collector"
	"ops-go/pkg/config"
	"ops-go/pkg/domain"
	"ops-go/pkg/orchestrator"
	"ops-go/pkg/notify"
	"ops-go/pkg/registry"
	"ops-go/pkg/report"
)

func newUpgradeCommand() *cobra.Command {
	var (
		dryRunFlag    bool
		yesFlag       bool
		onlyAutoFlag  bool
		forceFlag     bool
		notifyFlag    bool
		reportDirFlag string
		concurrency   int
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "批量扫描并受控执行全机（或自动组）可用镜像升级与留痕归档",
		Long: `upgrade 命令扫描所有受管容器在上游镜像仓库的新版本，
展示更新计划与安全级别，并在确认后按栈逐个执行前置快照门禁、Tag 改写、--no-deps 隔离重建与故障回滚。
操作完成后在 /var/lib/ops/reports/ 生成 Markdown 升级归档报告。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Minute)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cli, err := GetDockerClient(ctx)
			if err != nil {
				return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
			}

			cfg, err := GetConfig()
			if err != nil {
				cfg = &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}
			}

			col := collector.NewCollector(cli, cfg)
			allContainers, err := col.Collect(ctx)
			if err != nil {
				return fmt.Errorf("收集容器列表失败: %w", err)
			}

			var targets []collector.ContainerInfo
			for _, c := range allContainers {
				if onlyAutoFlag && !c.Auto && c.Guard != domain.GuardAuto {
					continue
				}
				if !forceFlag && (c.Guard == domain.GuardFrozen || c.Guard == domain.GuardLocked) {
					continue
				}
				targets = append(targets, c)
			}

			if len(targets) == 0 {
				pterm.Info.Println("未找到符合更新条件的容器")
				return nil
			}

			if concurrency <= 0 {
				concurrency = 5
			}

			pterm.DefaultHeader.WithFullWidth().Println("全机镜像升级扫描与受控流水线 (Batch Upgrade)")
			spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在并发检查 %d 个容器的新版本...", len(targets)))

			type upgradeTask struct {
				container collector.ContainerInfo
				candidate registry.UpdateCandidate
			}

			var (
				mu    sync.Mutex
				tasks []upgradeTask
			)

			regClient := registry.NewClient()
			sem := make(chan struct{}, concurrency)
			g, gctx := errgroup.WithContext(ctx)

			for _, c := range targets {
				c := c
				g.Go(func() error {
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-gctx.Done():
						return gctx.Err()
					}

					// 检查是否发生 Compose 配置漂移
					if c.HasDrift && c.DeclaredImage != "" {
						cand := registry.CompareImageRefs(c.ImageRef, c.DeclaredImage)
						mu.Lock()
						tasks = append(tasks, upgradeTask{
							container: c,
							candidate: cand,
						})
						mu.Unlock()
						return nil
					}

					var localDigest string
					imgInspect, err := cli.Engine().ImageInspect(gctx, c.ImageID)
					if err == nil && len(imgInspect.RepoDigests) > 0 {
						for _, rd := range imgInspect.RepoDigests {
							if strings.Contains(rd, "@sha256:") {
								parts := strings.Split(rd, "@")
								if len(parts) == 2 {
									localDigest = parts[1]
									break
								}
							}
						}
					}

					cand, checkErr := registry.CheckUpdate(gctx, regClient, c.ImageRef, localDigest)
					if checkErr == nil && cand.HasUpdate {
						mu.Lock()
						tasks = append(tasks, upgradeTask{
							container: c,
							candidate: cand,
						})
						mu.Unlock()
					}
					return nil
				})
			}

			_ = g.Wait()
			if spinner != nil {
				spinner.Stop()
			}

			if len(tasks) == 0 {
				pterm.DefaultBox.WithTitle("检查结果").Println(pterm.FgGreen.Sprint("所有容器镜像均已处于最新状态，无需升级。"))
				return nil
			}

			// 打印拟更新计划表
			planTable := pterm.TableData{
				{"SERVICE / STACK", "CURRENT TAG", "TARGET TAG", "TYPE", "GUARD", "REASON"},
			}
			for _, t := range tasks {
				c := t.container
				label := c.StackService()
				targetTag := t.candidate.TargetTag
				if t.candidate.UpdateType == registry.UpdateDigest {
					targetTag = fmt.Sprintf("%s (digest)", targetTag)
				}
				planTable = append(planTable, []string{
					pterm.Bold.Sprint(label),
					t.candidate.CurrentTag,
					pterm.FgGreen.Sprint(targetTag),
					string(t.candidate.UpdateType),
					string(c.Guard),
					t.candidate.Reason,
				})
			}

			pterm.DefaultSection.Println(fmt.Sprintf("待更新计划清单 (%d 个服务)", len(tasks)))
			_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(planTable).Render()
			pterm.Println()

			// 解析报告归档目录优先级: Flag > OPS_REPORT_DIR > ConfigFile > FHS标准(/var/log/ops)
			appCfg := GetAppConfig()
			var flagReportDir string
			if cmd.Flags().Changed("report-dir") {
				flagReportDir = reportDirFlag
			}
			targetReportDir := config.ResolvePrecedence(flagReportDir, os.Getenv("OPS_REPORT_DIR"), appCfg.ReportDir, "/var/log/ops")

			// 如果是 dry-run 模式
			startTime := time.Now()
			if dryRunFlag {
				pterm.Info.Println("[预演模式] 将不进行任何实际镜像拉取、配置修改与容器重启。")
				summary := report.UpgradeSummary{
					StartTime: startTime,
					EndTime:   time.Now(),
					DryRun:    true,
				}
				for _, t := range tasks {
					targetImg := t.candidate.TargetTag
					if t.container.HasDrift && t.container.DeclaredImage != "" {
						targetImg = t.container.DeclaredImage
					}
					summary.Items = append(summary.Items, report.ItemResult{
						Service:  t.container.Service,
						Stack:    t.container.Project,
						OldImage: t.container.ImageRef,
						NewImage: targetImg,
						Status:   report.StatusSuccess,
						Duration: 10 * time.Millisecond,
					})
				}
				reportPath, saveErr := summary.Save(targetReportDir)
				if saveErr == nil {
					pterm.Success.Printf("预演报告已生成: %s\n", reportPath)
				}
				return nil
			}

			// 前置权限门禁预检 (Preflight Check)
			targetStatePath := resolvedState
			if targetStatePath == "" {
				targetStatePath = "/var/lib/ops/state.json"
			}
			if err := config.CheckFileWritable(targetStatePath); err != nil {
				return err
			}
			if err := config.CheckDirWritable(targetReportDir); err != nil {
				// 若 /var/log/ops 权限受限，尝试平滑回退到 /var/lib/ops/reports
				if fallbackErr := config.CheckDirWritable("/var/lib/ops/reports"); fallbackErr == nil {
					targetReportDir = "/var/lib/ops/reports"
				} else {
					return err
				}
			}

			// 确认模式
			if !yesFlag {
				confirm, _ := pterm.DefaultInteractiveConfirm.WithDefaultText("是否立即执行上述受控更新流水线？").Show()
				if !confirm {
					pterm.Warning.Println("用户取消执行")
					return nil
				}
			}

			// 正式执行升级
			stateMgr := GetStateManager()
			orch := orchestrator.NewOrchestrator(cli, stateMgr)

			summary := report.UpgradeSummary{
				StartTime: startTime,
				DryRun:    false,
			}

			hasError := false
			for i, t := range tasks {
				targetLabel := t.container.StackService()
				pterm.DefaultHeader.WithFullWidth().Println(fmt.Sprintf("[%d/%d] 升级服务: %s", i+1, len(tasks), targetLabel))

				itemRes := report.ItemResult{
					Service:  t.container.Service,
					Stack:    t.container.Project,
					OldImage: t.container.ImageRef,
					NewImage: t.candidate.TargetTag,
				}

				opts := orchestrator.UpdateOptions{
					Tag:   t.candidate.TargetTag,
					Force: forceFlag,
				}
				if t.container.HasDrift && t.container.DeclaredImage != "" {
					opts.Image = t.container.DeclaredImage
					opts.Tag = ""
				}

				orchReport, updateErr := orch.UpdateContainer(ctx, &t.container, opts)
				if updateErr != nil {
					pterm.Error.Printf("%s 升级失败: %v\n", targetLabel, updateErr)
					itemRes.Status = report.StatusFailed
					itemRes.Error = updateErr.Error()
					hasError = true
				} else {
					itemRes.Duration = orchReport.Duration
					if orchReport.DumpPath != "" {
						itemRes.BackupFile = orchReport.DumpPath
					} else if orchReport.BackupPath != "" {
						itemRes.BackupFile = orchReport.BackupPath
					}

					switch orchReport.Status {
					case "rolled_back":
						pterm.Warning.Printf("%s 门禁未通过，已安全回滚: %s\n", targetLabel, orchReport.Message)
						itemRes.Status = report.StatusRolledBack
						itemRes.Error = orchReport.Message
						hasError = true
					case "skipped_identical":
						pterm.Info.Printf("%s 镜像未变动，跳过\n", targetLabel)
						itemRes.Status = report.StatusSkipped
					default:
						pterm.Success.Printf("%s 成功升级至 %s (耗时 %v)\n", targetLabel, orchReport.NewImageRef, orchReport.Duration.Round(time.Millisecond))
						itemRes.Status = report.StatusSuccess
					}
				}

				summary.Items = append(summary.Items, itemRes)
			}

			summary.EndTime = time.Now()
			reportPath, err := summary.Save(targetReportDir)
			if err == nil {
				_ = report.PruneReports(targetReportDir, 20)
				pterm.Println()
				pterm.Success.Printf("升级留痕报告已归档: %s\n", reportPath)
			} else {
				pterm.Warning.Printf("保存升级报告失败: %v\n", err)
			}

			if notifyFlag {
				notifyOpts := &notify.NotifyOptions{
					FeishuWebhook:   appCfg.Notify.FeishuWebhook,
					DingTalkWebhook: appCfg.Notify.DingTalkWebhook,
					WebhookURL:      appCfg.Notify.WebhookURL,
					TelegramToken:   appCfg.Notify.Telegram.BotToken,
					TelegramChatID:  appCfg.Notify.Telegram.ChatID,
					SMTPHost:        appCfg.Notify.SMTP.Host,
					SMTPPort:        appCfg.Notify.SMTP.Port,
					SMTPUser:        appCfg.Notify.SMTP.Username,
					SMTPPass:        appCfg.Notify.SMTP.Password,
					SMTPFrom:        appCfg.Notify.SMTP.From,
					SMTPTo:          appCfg.Notify.SMTP.To,
				}
				notifier := notify.NewFromEnvOrOptions(notifyOpts)
				if notifier != nil {
					pterm.Info.Println("正在向已配置渠道投递升级完成通知...")
					notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
					if sendErr := notifier.Send(notifyCtx, "ops upgrade 批量升级通知", summary.GenerateMarkdown()); sendErr != nil {
						pterm.Warning.Printf("通知投递失败: %v\n", sendErr)
					} else {
						pterm.Success.Println("通知已成功投递至各告警渠道")
					}
					notifyCancel()
				} else {
					pterm.Warning.Println("未检测到通知渠道配置（如 OPS_FEISHU_WEBHOOK、OPS_DINGTALK_WEBHOOK 或 SMTP_*）")
				}
			}

			if hasError {
				return fmt.Errorf("批量更新完成，但部分服务存在失败或回滚，请查阅报告: %s", reportPath)
			}

			pterm.DefaultBox.WithTitle("流水线完成").Println(pterm.FgGreen.Sprint("所有服务均已成功升级并通过健康校验！"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "仅预演更新流程，生成计划报告，不实际动刀")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "免确认直接执行升级 (适用于自动化或 Cron 调度)")
	cmd.Flags().BoolVar(&onlyAutoFlag, "only-auto", false, "仅升级配置为自动更新的容器 (wud.trigger 或 GuardAuto)")
	cmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "强制更新受保护的容器 (忽略 frozen / locked 限制)")
	cmd.Flags().BoolVar(&notifyFlag, "notify", false, "升级完成后自动向飞书/钉钉/Telegram/SMTP 发送通知报告")
	cmd.Flags().StringVar(&reportDirFlag, "report-dir", "/var/lib/ops/reports", "Markdown 升级留痕报告归档目录")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "j", 5, "远端镜像探测并发度")

	// 屏蔽非交互终端测试时的 panic
	if os.Getenv("CI") != "" || !isInteractiveTerminal() {
		// 允许在无 tty 且带 -y 时工作
	}

	return cmd
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
