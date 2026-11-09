package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"ops-go/pkg/collector"
	"ops-go/pkg/domain"
	"ops-go/pkg/registry"
)

type pendingResult struct {
	container   collector.ContainerInfo
	candidate   registry.UpdateCandidate
	localDigest string
	err         error
	skipped     string
}

func newPendingCommand() *cobra.Command {
	var showAll bool
	var concurrency int

	cmd := &cobra.Command{
		Use:   "pending",
		Short: "并发探测远端 Registry，检查全机容器是否存在可用新版本",
		Long: `pending 并发连接上游镜像仓库（Docker Hub、GHCR 或私有 Registry），
通过本地 docker 凭据存储（如 docker-credential-pass）自动鉴权，比对 SemVer 语义化标签与 Manifest 摘要，
并输出各服务当前的更新等级（Major / Minor / Patch / Digest）。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
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
			containers, err := col.Collect(ctx)
			if err != nil {
				return fmt.Errorf("收集容器状态失败: %w", err)
			}

			if concurrency <= 0 {
				concurrency = 5
			}

			regClient := registry.NewClient()

			pterm.DefaultHeader.WithFullWidth().Println("远端镜像仓库更新探测 (Registry Scanner)")
			spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在并发探测 %d 个容器的上游新版本...", len(containers)))

			var (
				mu      sync.Mutex
				results = make([]pendingResult, 0, len(containers))
			)

			sem := make(chan struct{}, concurrency)
			g, gctx := errgroup.WithContext(ctx)

			for _, c := range containers {
				c := c
				g.Go(func() error {
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-gctx.Done():
						return gctx.Err()
					}

					// 如果处于 frozen 或 locked 保护级别
					if c.Guard == domain.GuardFrozen {
						mu.Lock()
						results = append(results, pendingResult{
							container: c,
							skipped:   "已冻结更新 (frozen)",
						})
						mu.Unlock()
						return nil
					}
					if c.Guard == domain.GuardLocked {
						mu.Lock()
						results = append(results, pendingResult{
							container: c,
							skipped:   "已锁定通道 (locked)",
						})
						mu.Unlock()
						return nil
					}

					// 获取本地 image inspect 信息提取 RepoDigests
					var localDigest string
					imgInspect, err := cli.Engine().ImageInspect(gctx, c.ImageID)
					if err == nil && len(imgInspect.RepoDigests) > 0 {
						// 寻找最匹配当前 repo 的 digest，默认选第一个
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

					mu.Lock()
					results = append(results, pendingResult{
						container:   c,
						candidate:   cand,
						localDigest: localDigest,
						err:         checkErr,
					})
					mu.Unlock()
					return nil
				})
			}

			_ = g.Wait()
			if spinner != nil {
				spinner.Stop()
			}

			// 排序：有更新的排在前面，再按 stack / name 排序
			sort.Slice(results, func(i, j int) bool {
				iHas := results[i].candidate.HasUpdate
				jHas := results[j].candidate.HasUpdate
				if iHas != jHas {
					return iHas // 有更新的在先
				}
				if results[i].container.Project != results[j].container.Project {
					return results[i].container.Project < results[j].container.Project
				}
				return results[i].container.Name < results[j].container.Name
			})

			tableData := pterm.TableData{
				{"SERVICE / STACK", "CURRENT", "TARGET", "TYPE", "GUARD", "STATUS"},
			}

			updateCount := 0
			errorCount := 0

			for _, res := range results {
				c := res.container
				serviceStack := c.Name
				if c.Project != "" && c.Service != "" {
					serviceStack = fmt.Sprintf("%s / %s", c.Project, c.Service)
				}

				if res.skipped != "" {
					if showAll {
						tableData = append(tableData, []string{
							serviceStack,
							c.ImageRef,
							"-",
							"-",
							string(c.Guard),
							pterm.FgGray.Sprint(res.skipped),
						})
					}
					continue
				}

				if res.err != nil {
					errorCount++
					errMsg := res.err.Error()
					if len(errMsg) > 40 {
						errMsg = errMsg[:37] + "..."
					}
					tableData = append(tableData, []string{
						serviceStack,
						c.ImageRef,
						"-",
						"-",
						string(c.Guard),
						pterm.FgRed.Sprintf("探测失败: %s", errMsg),
					})
					continue
				}

				cand := res.candidate
				if cand.HasUpdate {
					updateCount++
					var typeFormatted string
					switch cand.UpdateType {
					case registry.UpdateMajor:
						typeFormatted = pterm.FgYellow.Sprint("MAJOR")
					case registry.UpdateMinor:
						typeFormatted = pterm.FgGreen.Sprint("MINOR")
					case registry.UpdatePatch:
						typeFormatted = pterm.FgCyan.Sprint("PATCH")
					case registry.UpdateDigest:
						typeFormatted = pterm.FgBlue.Sprint("DIGEST")
					default:
						typeFormatted = string(cand.UpdateType)
					}

					targetDisplay := cand.TargetTag
					if cand.UpdateType == registry.UpdateDigest {
						targetDisplay = fmt.Sprintf("%s (new sha)", cand.TargetTag)
					}

					tableData = append(tableData, []string{
						pterm.Bold.Sprint(serviceStack),
						cand.CurrentTag,
						pterm.Bold.Sprint(targetDisplay),
						typeFormatted,
						string(c.Guard),
						pterm.FgLightGreen.Sprint(cand.Reason),
					})
				} else if showAll {
					tableData = append(tableData, []string{
						serviceStack,
						cand.CurrentTag,
						cand.TargetTag,
						pterm.FgGray.Sprint("UP-TO-DATE"),
						string(c.Guard),
						pterm.FgGray.Sprint("已是最新"),
					})
				}
			}

			if updateCount == 0 && !showAll && errorCount == 0 {
				pterm.DefaultBox.WithTitle("探测结果").Println(pterm.FgGreen.Sprintf("全机 %d 个容器均已处于最新状态，无需更新。", len(containers)))
				return nil
			}

			_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()

			pterm.Println()
			summaryMsg := fmt.Sprintf("已扫描容器: %d  |  可用更新: %d  |  探测异常: %d", len(containers), updateCount, errorCount)
			if updateCount > 0 {
				pterm.DefaultBox.WithTitle("操作指引").Println(
					fmt.Sprintf("%s\n- 单独更新指定服务: %s\n- 批量全量升级:     %s",
						pterm.Bold.Sprint(summaryMsg),
						pterm.FgCyan.Sprint("ops update <service>"),
						pterm.FgLightGreen.Sprint("ops upgrade [--dry-run]"),
					),
				)
			} else {
				pterm.Info.Println(summaryMsg)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "显示全部容器（包含暂无更新和已锁定的容器）")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "j", 5, "并发探测任务数")

	return cmd
}
