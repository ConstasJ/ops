package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func newPingCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "探测并检查 Docker Daemon 的 Socket 连通性与版本信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			cli, err := GetDockerClient(ctx)
			if err != nil {
				pterm.Error.Printfln("无法初始化 Docker 客户端: %v", err)
				return err
			}

			// Ping daemon
			ping, err := cli.Ping(ctx)
			if err != nil {
				pterm.Error.Printfln("Docker Daemon 连通性探测失败: %v", err)
				return err
			}

			// Retrieve version details
			ver, err := cli.ServerVersion(ctx)
			if err != nil {
				pterm.Error.Printfln("获取 Docker Server 版本信息失败: %v", err)
				return err
			}

			pterm.Success.Printfln("成功连接至 Docker Daemon (API Version: %s)", ping.APIVersion)

			platform := ver.Platform.Name
			if platform == "" {
				platform = "Docker Engine"
			}

			tableData := pterm.TableData{
				{"属性", "值"},
				{"平台 / 产品", platform},
				{"引擎版本", ver.Version},
				{"API 版本", fmt.Sprintf("%s (最低支持: %s)", ver.APIVersion, ver.MinAPIVersion)},
				{"操作系统 / 架构", fmt.Sprintf("%s / %s", ver.Os, ver.Arch)},
			}

			for _, comp := range ver.Components {
				tableData = append(tableData, []string{
					fmt.Sprintf("组件: %s", comp.Name),
					comp.Version,
				})
			}

			_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
			return nil
		},
	}
}
