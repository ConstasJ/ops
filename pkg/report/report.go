package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ItemStatus 升级执行状态
type ItemStatus string

const (
	StatusSuccess    ItemStatus = "success"
	StatusFailed     ItemStatus = "failed"
	StatusRolledBack ItemStatus = "rolled_back"
	StatusSkipped    ItemStatus = "skipped"
)

// ItemResult 单个服务的升级结果
type ItemResult struct {
	Service    string        `json:"service"`
	Stack      string        `json:"stack"`
	OldImage   string        `json:"old_image"`
	NewImage   string        `json:"new_image"`
	Status     ItemStatus    `json:"status"`
	Duration   time.Duration `json:"duration"`
	BackupFile string        `json:"backup_file,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// UpgradeSummary 一次升级批处理的整体汇总
type UpgradeSummary struct {
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	DryRun    bool         `json:"dry_run"`
	Host      string       `json:"host"`
	Items     []ItemResult `json:"items"`
}

// GenerateMarkdown 生成结构化 Markdown 留痕报告
func (s *UpgradeSummary) GenerateMarkdown() string {
	var sb strings.Builder

	duration := s.EndTime.Sub(s.StartTime).Round(time.Millisecond)

	successCount := 0
	failedCount := 0
	rollbackCount := 0
	skippedCount := 0

	for _, item := range s.Items {
		switch item.Status {
		case StatusSuccess:
			successCount++
		case StatusFailed:
			failedCount++
		case StatusRolledBack:
			rollbackCount++
		case StatusSkipped:
			skippedCount++
		}
	}

	modeStr := "正式执行"
	if s.DryRun {
		modeStr = "预演模式 (Dry-Run)"
	}

	hostStr := s.Host
	if hostStr == "" {
		hostStr, _ = os.Hostname()
	}

	sb.WriteString(fmt.Sprintf("# ops upgrade 批量受控升级报告\n\n"))
	sb.WriteString(fmt.Sprintf("> 模式: **%s** | 宿主: `%s` | 生成时间: `%s`\n\n",
		modeStr, hostStr, s.EndTime.Format("2006-01-02 15:04:05 MST")))

	sb.WriteString("## 1. 总体指标\n\n")
	sb.WriteString("| 指标项 | 统计值 |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString(fmt.Sprintf("| 开始时间 | %s |\n", s.StartTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("| 结束时间 | %s |\n", s.EndTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("| 批处理总耗时 | %s |\n", duration))
	sb.WriteString(fmt.Sprintf("| 总升级服务数 | %d |\n", len(s.Items)))
	sb.WriteString(fmt.Sprintf("| 成功 | %d |\n", successCount))
	sb.WriteString(fmt.Sprintf("| 失败 | %d |\n", failedCount))
	sb.WriteString(fmt.Sprintf("| 触发回滚 | %d |\n", rollbackCount))
	sb.WriteString(fmt.Sprintf("| 跳过 | %d |\n\n", skippedCount))

	sb.WriteString("## 2. 服务变更明细\n\n")
	sb.WriteString("| 项目/栈 | 服务名 | 原镜像 | 目标镜像 | 状态 | 耗时 | 备注/快照 |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")

	for _, item := range s.Items {
		statusBadge := ""
		switch item.Status {
		case StatusSuccess:
			statusBadge = "✅ 成功"
		case StatusFailed:
			statusBadge = "❌ 失败"
		case StatusRolledBack:
			statusBadge = "⚠️ 已自动回滚"
		case StatusSkipped:
			statusBadge = "⏸️ 跳过"
		default:
			statusBadge = string(item.Status)
		}

		note := "-"
		if item.BackupFile != "" {
			note = fmt.Sprintf("快照: `%s`", filepath.Base(item.BackupFile))
		}
		if item.Error != "" {
			if note != "-" {
				note += " | "
			} else {
				note = ""
			}
			note += fmt.Sprintf("错误: %s", item.Error)
		}

		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | %s | %s | %s |\n",
			item.Stack,
			item.Service,
			item.OldImage,
			item.NewImage,
			statusBadge,
			item.Duration.Round(time.Millisecond),
			note,
		))
	}

	sb.WriteString("\n## 3. 运维后置指引\n\n")
	if failedCount > 0 || rollbackCount > 0 {
		sb.WriteString("> [!WARNING]\n")
		sb.WriteString("> 本次更新存在失败或已自动回滚的服务，请运行 `ops status` 查看容器实时健康度，或检查 `/var/backups/ops/` 下的快照档案。\n\n")
	} else {
		sb.WriteString("所有受管服务均已安全升级并通过健康探针验证。系统运行平稳。\n\n")
	}

	return sb.String()
}

// Save 将报告保存到指定目录（默认 /var/lib/ops/reports），并返回报告文件的绝对路径
func (s *UpgradeSummary) Save(dir string) (string, error) {
	if dir == "" {
		dir = "/var/lib/ops/reports"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建报告目录失败: %w", err)
	}

	filename := fmt.Sprintf("upgrade-%s.md", s.StartTime.Format("20060102-150405"))
	targetPath := filepath.Join(dir, filename)

	content := s.GenerateMarkdown()
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入报告文件失败: %w", err)
	}

	return targetPath, nil
}

// PruneReports 扫描目录中的 upgrade-*.md 报告文件，按时间倒序保留最新的 maxKeep 份，清理超期历史
func PruneReports(dir string, maxKeep int) error {
	if maxKeep <= 0 {
		maxKeep = 20
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var reportFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "upgrade-") && strings.HasSuffix(e.Name(), ".md") {
			reportFiles = append(reportFiles, e.Name())
		}
	}

	// 倒序排序（根据文件名的时间戳命名，降序即最新的在前）
	sort.Slice(reportFiles, func(i, j int) bool {
		return reportFiles[i] > reportFiles[j]
	})

	if len(reportFiles) <= maxKeep {
		return nil
	}

	for _, f := range reportFiles[maxKeep:] {
		_ = os.Remove(filepath.Join(dir, f))
	}

	return nil
}
