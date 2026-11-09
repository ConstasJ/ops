package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/orchestrator"
)

func newRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback <目标>",
		Short: "依据 state.json 历史记录，一键将容器与 Compose 文件原子回滚至上一稳定版本",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cli, err := GetDockerClient(ctx)
			if err != nil {
				return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
			}

			stateMgr := GetStateManager()
			orch := orchestrator.NewOrchestrator(cli, stateMgr)

			pterm.DefaultHeader.WithFullWidth().Println(fmt.Sprintf("开始执行一键回滚: %s", target))
			spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在查找并回退 %s...", target))

			report, err := orch.Rollback(ctx, target)
			if err != nil {
				spinner.Fail(fmt.Sprintf("%s 回滚失败: %v", target, err))
				return err
			}

			spinner.Success(fmt.Sprintf("容器 %s 已成功回退至 %s (耗时 %v)", target, report.RestoredRef, report.Duration.Round(time.Millisecond)))
			return nil
		},
	}

	return cmd
}
