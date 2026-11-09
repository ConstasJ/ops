package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/collector"
	"ops-go/pkg/domain"
)

var (
	lsManualFlag bool
	lsAutoFlag   bool
	lsAllFlag    bool
)

func newLsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出受控容器及其类型、保护级别与注意事项",
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
				cfg = &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}
			}

			col := collector.NewCollector(cli, cfg)
			containers, err := col.Collect(ctx)
			if err != nil {
				return fmt.Errorf("收集容器状态失败: %w", err)
			}

			// Filtering logic:
			// If --auto is set: filter auto only
			// If --all is set: show all
			// Default or --manual: show manual only (where !c.Auto)
			var filtered []collector.ContainerInfo
			for _, c := range containers {
				if lsAutoFlag {
					if c.Auto {
						filtered = append(filtered, c)
					}
				} else if lsAllFlag {
					filtered = append(filtered, c)
				} else {
					// manual is default
					if !c.Auto {
						filtered = append(filtered, c)
					}
				}
			}

			filterTitle := "受控容器列表 (Manual 组)"
			if lsAutoFlag {
				filterTitle = "WUD 自动更新组容器列表"
			} else if lsAllFlag {
				filterTitle = "全量容器列表"
			}

			pterm.DefaultHeader.WithFullWidth().Println(filterTitle)

			tableData := pterm.TableData{
				{"容器名称", "栈 / 服务", "类型", "保护级别", "健康状态", "注意事项说明"},
			}

			for _, c := range filtered {
				notePreview := "-"
				if c.Notes != "" {
					firstLine := strings.Split(c.Notes, "\n")[0]
					if len([]rune(firstLine)) > 35 {
						notePreview = string([]rune(firstLine)[:32]) + "..."
					} else {
						notePreview = firstLine
					}
				}

				tableData = append(tableData, []string{
					c.Name,
					c.StackService(),
					c.Kind,
					formatGuard(c.Guard),
					formatHealth(c.Health),
					pterm.Gray(notePreview),
				})
			}

			_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
			return nil
		},
	}

	cmd.Flags().BoolVar(&lsManualFlag, "manual", false, "仅列出人工值守的受控容器 (默认行为)")
	cmd.Flags().BoolVar(&lsAutoFlag, "auto", false, "仅列出 WUD 自动更新白名单内的容器")
	cmd.Flags().BoolVar(&lsAllFlag, "all", false, "列出全部容器 (包含 auto 与 manual)")

	return cmd
}
