package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/collector"
	"ops-go/pkg/domain"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看全机容器运行状态、健康门禁与重启指标",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cli, err := GetDockerClient(ctx)
			if err != nil {
				return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
			}

			cfg, err := GetConfig()
			if err != nil {
				// Non-fatal, fallback to empty
				cfg = &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}
			}

			col := collector.NewCollector(cli, cfg)
			containers, err := col.Collect(ctx)
			if err != nil {
				return fmt.Errorf("收集容器状态失败: %w", err)
			}

			// Aggregate stats
			runningCount := 0
			unhealthyCount := 0
			restartCount := 0
			autoCount := 0
			stoppedCount := 0

			for _, c := range containers {
				if c.State == "running" {
					runningCount++
				} else {
					stoppedCount++
				}
				if c.Health == "unhealthy" {
					unhealthyCount++
				}
				if c.RestartCount > 0 {
					restartCount++
				}
				if c.Auto {
					autoCount++
				}
			}

			pterm.DefaultHeader.WithFullWidth().Println("Ounasjoki 全机容器状态总览")

			summaryText := fmt.Sprintf(
				"容器总数: %d  |  运行中: %s  |  已停止: %s  |  不健康: %s  |  发生过重启: %s  |  自动更新组: %d",
				len(containers),
				pterm.Green(runningCount),
				formatStopped(stoppedCount),
				formatUnhealthy(unhealthyCount),
				formatRestarts(restartCount),
				autoCount,
			)
			pterm.DefaultBox.Println(summaryText)

			tableData := pterm.TableData{
				{"容器名称", "栈 / 服务", "状态", "健康门禁", "重启数", "运行时长", "保护级别"},
			}

			for _, c := range containers {
				stateStr := formatState(c.State)
				healthStr := formatHealth(c.Health)
				guardStr := formatGuard(c.Guard)
				restartsStr := fmt.Sprintf("%d", c.RestartCount)
				if c.RestartCount > 0 {
					restartsStr = pterm.Yellow(c.RestartCount)
				}

				tableData = append(tableData, []string{
					c.Name,
					c.StackService(),
					stateStr,
					healthStr,
					restartsStr,
					c.FormatUptime(),
					guardStr,
				})
			}

			_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
			return nil
		},
	}
}

func formatStopped(n int) string {
	if n > 0 {
		return pterm.Red(n)
	}
	return pterm.Green("0")
}

func formatUnhealthy(n int) string {
	if n > 0 {
		return pterm.Red(n)
	}
	return pterm.Green("0")
}

func formatRestarts(n int) string {
	if n > 0 {
		return pterm.Yellow(n)
	}
	return "0"
}

func formatState(state string) string {
	switch state {
	case "running":
		return pterm.Green("running")
	case "exited":
		return pterm.Red("exited")
	case "restarting":
		return pterm.Yellow("restarting")
	default:
		return pterm.Gray(state)
	}
}

func formatHealth(health string) string {
	switch health {
	case "healthy":
		return pterm.Green("healthy")
	case "unhealthy":
		return pterm.Red("unhealthy")
	case "starting":
		return pterm.Yellow("starting")
	case "":
		return pterm.Gray("-")
	default:
		return health
	}
}

func formatGuard(g domain.GuardLevel) string {
	switch g {
	case domain.GuardAuto:
		return pterm.Blue("auto")
	case domain.GuardDB:
		return pterm.Magenta("db")
	case domain.GuardFrozen:
		return pterm.Red("frozen")
	case domain.GuardLocked:
		return pterm.Yellow("locked")
	default:
		return pterm.Cyan("assisted")
	}
}
