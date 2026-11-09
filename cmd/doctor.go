package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/collector"
	"ops-go/pkg/doctor"
	"ops-go/pkg/domain"
)

var doctorBackupRootFlag string

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "系统健康全面体检：容器异常退出、unhealthy 门禁、高频重启与快照新鲜度",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
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
				return fmt.Errorf("获取容器列表失败: %w", err)
			}

			chk := doctor.NewChecker(doctorBackupRootFlag, cli)

			pterm.DefaultHeader.WithFullWidth().Println("Ounasjoki 系统健康全面巡检 (Doctor)")

			// 1. Container status checks
			pterm.DefaultSection.Println("1. 容器运行状态与健康门禁体检")
			cReport := chk.CheckContainers(containers)
			if len(cReport.Issues) == 0 {
				pterm.Success.Println("所有运行中容器健康检查全部正常，未发现异常退出或频繁重启。")
			} else {
				for _, iss := range cReport.Issues {
					msg := fmt.Sprintf("[%s] %s: %s", iss.Container, iss.Title, iss.Detail)
					if iss.Level == doctor.LevelError {
						pterm.Error.Println(msg)
					} else {
						pterm.Warning.Println(msg)
					}
				}
			}

			// 2. Backup freshness checks
			pterm.DefaultSection.Println("2. 核心数据库快照新鲜度体检 (Guard: DB)")
			bReport := chk.CheckBackups(containers)
			if len(bReport.Statuses) == 0 {
				pterm.Info.Println("当前未发现配置了 Guard: DB 保护级别的数据库容器。")
			} else {
				tableData := pterm.TableData{
					{"目标容器", "存储类型", "状态", "快照详情"},
				}
				for _, st := range bReport.Statuses {
					statusBadge := pterm.Green("OK")
					if st.Level == doctor.LevelError {
						statusBadge = pterm.Red("ERROR")
					} else if st.Level == doctor.LevelWarn {
						statusBadge = pterm.Yellow("WARN")
					}
					tableData = append(tableData, []string{
						st.Container,
						st.Kind,
						statusBadge,
						st.Detail,
					})
				}
				_ = pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
			}

			// Overall Summary
			totalErrors := cReport.ErrorCount + bReport.ErrorCount
			totalWarns := cReport.WarnCount + bReport.WarnCount

			pterm.Println()
			if totalErrors > 0 {
				pterm.Error.Printfln("巡检结论：发现 %d 项严重异常，%d 项告警，请重点排查！", totalErrors, totalWarns)
			} else if totalWarns > 0 {
				pterm.Warning.Printfln("巡检结论：发现 %d 项告警关注项，未发现严重阻断性故障。", totalWarns)
			} else {
				pterm.Success.Println("巡检结论：系统整体运行健康，服务与快照状态全绿！")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&doctorBackupRootFlag, "backup-root", "/var/backups/ops", "数据库快照根目录路径")
	return cmd
}
