package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMarkdownReport(t *testing.T) {
	summary := &UpgradeSummary{
		StartTime: time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 11, 9, 18, 2, 30, 0, time.UTC),
		DryRun:    false,
		Host:      "ounasjoki",
		Items: []ItemResult{
			{
				Service:    "redis",
				Stack:      "cache-stack",
				OldImage:   "redis:7.2.3",
				NewImage:   "redis:7.2.4",
				Status:     StatusSuccess,
				Duration:   15 * time.Second,
				BackupFile: "/var/backups/ops/redis-20261109.zst",
			},
			{
				Service:  "gitea",
				Stack:    "gitea-stack",
				OldImage: "gitea/gitea:1.21.0",
				NewImage: "gitea/gitea:1.21.1",
				Status:   StatusFailed,
				Duration: 45 * time.Second,
				Error:    "health probe timeout after 30s",
			},
		},
	}

	content := summary.GenerateMarkdown()
	assert.Contains(t, content, "# ops upgrade 批量受控升级报告")
	assert.Contains(t, content, "ounasjoki")
	assert.Contains(t, content, "redis")
	assert.Contains(t, content, "gitea")
	assert.Contains(t, content, "health probe timeout")
	assert.Contains(t, content, "成功")
	assert.Contains(t, content, "失败")
}

func TestSaveAndPruneReports(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ops-reports-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建 25 个模拟历史报告文件
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 25; i++ {
		filename := filepath.Join(tmpDir, baseTime.Add(time.Duration(i)*time.Hour).Format("upgrade-20060102-150405.md"))
		err := os.WriteFile(filename, []byte("# Dummy Report"), 0644)
		require.NoError(t, err)
	}

	// 验证初始数量为 25
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 25, len(files))

	// 保留最新 20 份
	err = PruneReports(tmpDir, 20)
	require.NoError(t, err)

	filesAfter, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 20, len(filesAfter))
}
