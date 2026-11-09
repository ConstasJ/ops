package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/orchestrator"
)

var (
	updateTagFlag    string
	updateImageFlag  string
	updateForceFlag  bool
	updateDryRunFlag bool
)

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <目标...>",
		Short: "对受控容器执行前置快照门禁、Tag 改写、--no-deps 隔离重建与自动回滚保护的安全更新",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cli, err := GetDockerClient(ctx)
			if err != nil {
				return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
			}

			stateMgr := GetStateManager()
			orch := orchestrator.NewOrchestrator(cli, stateMgr)

			opts := orchestrator.UpdateOptions{
				Tag:    updateTagFlag,
				Image:  updateImageFlag,
				Force:  updateForceFlag,
				DryRun: updateDryRunFlag,
			}

			hasError := false
			for _, target := range args {
				pterm.DefaultHeader.WithFullWidth().Println(fmt.Sprintf("开始执行受控更新: %s", target))
				spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在处理 %s...", target))

				report, err := orch.Update(ctx, target, opts)
				if err != nil {
					spinner.Fail(fmt.Sprintf("%s 更新失败: %v", target, err))
					hasError = true
					continue
				}

				switch report.Status {
				case "skipped_identical":
					spinner.Warning(fmt.Sprintf("%s 镜像已是最新，跳过更新", target))
				case "dry_run":
					spinner.Info(fmt.Sprintf("[预演] %s -> %s", report.OldImageRef, report.NewImageRef))
				case "rolled_back":
					spinner.Fail(fmt.Sprintf("%s 自动回滚完成: %s", target, report.Message))
					hasError = true
				default:
					spinner.Success(fmt.Sprintf("%s 成功更新至 %s (耗时 %v)", target, report.NewImageRef, report.Duration.Round(time.Millisecond)))
				}
			}

			if hasError {
				return fmt.Errorf("部分容器更新失败或已自动回滚")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&updateTagFlag, "tag", "t", "", "指定目标镜像 Tag (例如: 1.25-alpine)")
	cmd.Flags().StringVarP(&updateImageFlag, "image", "i", "", "指定完整目标镜像 (例如: redis:7.2-alpine)")
	cmd.Flags().BoolVarP(&updateForceFlag, "force", "f", false, "强制更新受保护的容器 (忽略 frozen / locked 限制)")
	cmd.Flags().BoolVar(&updateDryRunFlag, "dry-run", false, "仅预演更新流程，不修改文件与容器")

	return cmd
}
