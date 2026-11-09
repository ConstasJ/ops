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

func newNotesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "notes [容器名]",
		Short: "查看容器的人工注意事项、踩坑档案与探针配置",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cfg, err := GetConfig()
			if err != nil {
				cfg = &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}
			}

			// If specific container is requested
			if len(args) == 1 {
				target := args[0]
				cli, err := GetDockerClient(ctx)
				if err != nil {
					return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
				}
				col := collector.NewCollector(cli, cfg)
				c, err := col.Find(ctx, target)
				if err != nil {
					return err
				}

				renderContainerNotes(c)
				return nil
			}

			// If no argument is provided: list all containers with notes or special guards
			cli, err := GetDockerClient(ctx)
			if err != nil {
				return fmt.Errorf("无法连接 Docker 守护进程: %w", err)
			}
			col := collector.NewCollector(cli, cfg)
			containers, err := col.Collect(ctx)
			if err != nil {
				return fmt.Errorf("获取容器列表失败: %w", err)
			}

			pterm.DefaultHeader.WithFullWidth().Println("容器人工值守注意事项与排坑档案")

			count := 0
			for _, c := range containers {
				if c.Notes != "" || c.Guard == domain.GuardFrozen || c.Guard == domain.GuardLocked {
					count++
					renderContainerNotes(&c)
				}
			}

			if count == 0 {
				pterm.Info.Println("当前知识库中暂无可展示的注意事项。")
			}
			return nil
		},
	}
}

func renderContainerNotes(c *collector.ContainerInfo) {
	guardBadge := formatGuard(c.Guard)
	title := fmt.Sprintf("%s (%s) — 保护级别: %s", c.Name, c.StackService(), guardBadge)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("服务类型:  %s\n", c.Kind))
	sb.WriteString(fmt.Sprintf("超时控制:  门禁超时 %v, 稳定观察 %v\n", c.Timeout, c.Settle))

	if c.Probe != nil {
		if c.Probe.HTTP != "" {
			insecureStr := ""
			if c.Probe.Insecure {
				insecureStr = " (insecure)"
			}
			sb.WriteString(fmt.Sprintf("存活探针:  HTTP %s%s\n", c.Probe.HTTP, insecureStr))
		} else if c.Probe.TCP != "" {
			sb.WriteString(fmt.Sprintf("存活探针:  TCP %s\n", c.Probe.TCP))
		}
	} else {
		sb.WriteString("存活探针:  无自定义探针 (依赖容器健康检查或稳定期)\n")
	}

	sb.WriteString("\n注意事项与避坑手册:\n")
	if c.Notes != "" {
		for _, line := range strings.Split(c.Notes, "\n") {
			sb.WriteString(fmt.Sprintf("  • %s\n", line))
		}
	} else {
		sb.WriteString("  (无人工填写注意事项)\n")
	}

	pterm.DefaultBox.WithTitle(title).Println(sb.String())
}
