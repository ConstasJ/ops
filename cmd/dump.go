package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"ops-go/pkg/collector"
	"ops-go/pkg/config"
	"ops-go/pkg/domain"
	"ops-go/pkg/dumper"
)

var (
	dumpFullFlag       bool
	dumpBackupRootFlag string
	dumpKeepFlag       int
)

func newDumpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump <容器名>",
		Short: "对数据库/有状态容器执行零宿主依赖的原生快照并流式 zstd 压缩落盘",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
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
			c, err := col.Find(ctx, target)
			if err != nil {
				return err
			}

			// Validate kind
			var d dumper.Dumper
			if c.Kind == "gitea" {
				gd := dumper.NewGiteaDumper(cli)
				gd.Full = dumpFullFlag
				d = gd
			} else {
				d, err = dumper.GetDumper(c.Kind, cli)
				if err != nil {
					return fmt.Errorf("容器 %s 类型为 %q，非支持的原生数据库快照类型: %w", c.Name, c.Kind, err)
				}
			}

			appCfg := GetAppConfig()
			var flagBackupDir string
			if cmd.Flags().Changed("out") {
				flagBackupDir = dumpBackupRootFlag
			}
			targetBackupRoot := config.ResolvePrecedence(flagBackupDir, os.Getenv("OPS_BACKUP_DIR"), appCfg.BackupDir, "/var/backups/ops")

			if err := config.CheckDirWritable(targetBackupRoot); err != nil {
				return err
			}

			pterm.DefaultHeader.WithFullWidth().Println(fmt.Sprintf("开始执行容器数据快照: %s", c.Name))
			spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在备份 %s (%s)...", c.Name, c.Kind))

			outDir, err := dumper.CreateBackupDir(targetBackupRoot, c.Name)
			if err != nil {
				spinner.Fail(fmt.Sprintf("创建备份目录失败: %v", err))
				return err
			}

			res, err := d.Dump(ctx, c, outDir)
			if err != nil {
				spinner.Fail(fmt.Sprintf("快照执行失败: %v", err))
				return err
			}

			// Write meta.json
			meta := dumper.MetaJSON{
				Container:     c.Name,
				At:            time.Now().Format("2006-01-02T15:04:05"),
				Kind:          c.Kind,
				ImageBefore:   c.ImageRef,
				ImageIDBefore: c.ImageID,
				What:          res.Description,
				DurationSec:   res.Duration.Seconds(),
				SizeBytes:     res.SizeBytes,
			}
			if err := dumper.WriteMetaJSON(outDir, meta); err != nil {
				spinner.Warning(fmt.Sprintf("写入 meta.json 失败: %v", err))
			}

			// Rotate backups
			_ = dumper.RotateBackups(dumpBackupRootFlag, c.Name, dumpKeepFlag)

			spinner.Success("快照创建成功并完成完整性校验！")

			pterm.DefaultBox.WithTitle("快照产物元数据").Sprint(
				fmt.Sprintf("容器目标:   %s (%s)\n存放路径:   %s\n产物描述:   %s\n产物体积:   %s (共 %d 字节)\n执行耗时:   %v\n保留策略:   最新 %d 份",
					res.Container,
					res.Kind,
					res.OutputDir,
					res.Description,
					formatBytes(res.SizeBytes),
					res.SizeBytes,
					res.Duration.Round(time.Millisecond),
					dumpKeepFlag,
				),
			)

			pterm.Println()
			return nil
		},
	}

	cmd.Flags().BoolVar(&dumpFullFlag, "full", false, "对于 Gitea 执行全量打包 (包含仓库、LFS 与附件)")
	cmd.Flags().StringVar(&dumpBackupRootFlag, "backup-root", "/var/backups/ops", "快照根目录路径")
	cmd.Flags().IntVar(&dumpKeepFlag, "keep", 5, "历史快照保留份数 (默认 5 份)")

	return cmd
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
