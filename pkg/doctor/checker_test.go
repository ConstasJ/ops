package doctor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/collector"
	"ops-go/pkg/doctor"
	"ops-go/pkg/domain"
)

func TestChecker_DiagnoseContainers(t *testing.T) {
	containers := []collector.ContainerInfo{
		{Name: "healthy-app", State: "running", Health: "healthy", RestartCount: 0},
		{Name: "unhealthy-app", State: "running", Health: "unhealthy", RestartCount: 1},
		{Name: "stopped-app", State: "exited", ExitCode: 137},
		{Name: "flapping-app", State: "running", RestartCount: 12},
	}

	chk := doctor.NewChecker("", nil)
	report := chk.CheckContainers(containers)

	assert.Equal(t, 3, len(report.Issues)) // unhealthy, stopped, flapping
	assert.Equal(t, 2, report.ErrorCount)   // unhealthy + stopped
	assert.Equal(t, 1, report.WarnCount)    // flapping
}

func TestChecker_BackupFreshness(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fresh backup for db-1 (1 hour ago, format 20060102-150405)
	freshDir := filepath.Join(tmpDir, "db-1", time.Now().Add(-1*time.Hour).Format("20060102-150405"))
	require.NoError(t, os.MkdirAll(freshDir, 0755))

	// Create stale backup for db-2 (72 hours ago)
	staleDir := filepath.Join(tmpDir, "db-2", time.Now().Add(-72*time.Hour).Format("20060102-150405"))
	require.NoError(t, os.MkdirAll(staleDir, 0755))

	containers := []collector.ContainerInfo{
		{Name: "db-1", Guard: domain.GuardDB, Kind: "postgres"},
		{Name: "db-2", Guard: domain.GuardDB, Kind: "redis"},
		{Name: "db-3", Guard: domain.GuardDB, Kind: "sqlite"}, // No backup directory at all
	}

	chk := doctor.NewChecker(tmpDir, nil)
	report := chk.CheckBackups(containers)

	require.Len(t, report.Statuses, 3)
	// db-1: fresh -> ok
	assert.Equal(t, doctor.LevelOK, report.Statuses[0].Level)
	// db-2: stale -> warn
	assert.Equal(t, doctor.LevelWarn, report.Statuses[1].Level)
	// db-3: missing -> error
	assert.Equal(t, doctor.LevelError, report.Statuses[2].Level)
}
