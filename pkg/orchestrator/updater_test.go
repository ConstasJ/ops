package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ops-go/pkg/collector"
	"ops-go/pkg/compose"
	"ops-go/pkg/domain"
	"ops-go/pkg/dumper"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleComposeContent = `services:
  web:
    image: nginx:1.24-alpine
  db:
    image: postgres:15-alpine
`

type mockDumper struct {
	dumpErr error
	dumped  bool
}

func (m *mockDumper) Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*dumper.DumpResult, error) {
	m.dumped = true
	if m.dumpErr != nil {
		return nil, m.dumpErr
	}
	return &dumper.DumpResult{
		OutputDir: outDir,
		Artifacts: []string{filepath.Join(outDir, "dump.sql.zst")},
		SizeBytes: 1024,
		Duration:  10 * time.Millisecond,
	}, nil
}

func setupTestEnv(t *testing.T) (string, string, *domain.StateManager) {
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composePath, []byte(sampleComposeContent), 0644))

	statePath := filepath.Join(tmpDir, "state.json")
	stateMgr := domain.NewStateManager(statePath)
	return tmpDir, composePath, stateMgr
}

func TestOrchestrator_GuardLevelEnforcement(t *testing.T) {
	_, composePath, stateMgr := setupTestEnv(t)

	mockContainer := collector.ContainerInfo{
		Name:     "nextskin",
		ImageRef: "nextskin/nextskin:1.0",
		Guard:    domain.GuardFrozen,
		Files:    []string{composePath},
		Service:  "web",
	}

	finder := func(ctx context.Context, target string) (*collector.ContainerInfo, error) {
		return &mockContainer, nil
	}

	orch := NewOrchestrator(nil, stateMgr).WithContainerFinder(finder)

	t.Run("Frozen container update rejected without force", func(t *testing.T) {
		report, err := orch.Update(context.Background(), "nextskin", UpdateOptions{})
		require.Error(t, err)
		assert.Equal(t, "refused", report.Status)
		assert.Contains(t, err.Error(), "frozen")
	})

	t.Run("Frozen container update proceeds with force", func(t *testing.T) {
		orch.imageInspector = func(ctx context.Context, ref string) (string, error) {
			return "sha256:newid", nil
		}
		orch.imagePuller = func(ctx context.Context, ref string) error { return nil }
		orch.composeUpper = func(ctx context.Context, composeFile string, service string) error { return nil }
		orch.waiter = func(ctx context.Context, c *collector.ContainerInfo) (bool, string) { return true, "healthy" }

		report, err := orch.Update(context.Background(), "nextskin", UpdateOptions{Force: true, Tag: "1.1"})
		require.NoError(t, err)
		assert.Equal(t, "success", report.Status)
	})
}

func TestOrchestrator_GuardDBSnapshotGate(t *testing.T) {
	_, composePath, stateMgr := setupTestEnv(t)

	mockContainer := collector.ContainerInfo{
		Name:     "app-db",
		ImageRef: "postgres:15-alpine",
		Guard:    domain.GuardDB,
		Kind:     "postgres",
		Files:    []string{composePath},
		Service:  "db",
	}

	finder := func(ctx context.Context, target string) (*collector.ContainerInfo, error) {
		return &mockContainer, nil
	}

	t.Run("Aborts update when snapshot fails", func(t *testing.T) {
		dumperMock := &mockDumper{dumpErr: errors.New("disk full")}

		orch := NewOrchestrator(nil, stateMgr).
			WithContainerFinder(finder).
			WithDumper(dumperMock)

		orch.imagePuller = func(ctx context.Context, ref string) error { return nil }
		orch.imageInspector = func(ctx context.Context, ref string) (string, error) { return "sha256:new", nil }

		report, err := orch.Update(context.Background(), "app-db", UpdateOptions{Tag: "16-alpine"})
		require.Error(t, err)
		assert.Equal(t, "dump_failed", report.Status)
		assert.True(t, dumperMock.dumped)

		// Verify compose file was NOT changed
		content, _ := os.ReadFile(composePath)
		assert.Contains(t, string(content), "postgres:15-alpine")
	})
}

func TestOrchestrator_IdempotentSkip(t *testing.T) {
	_, composePath, stateMgr := setupTestEnv(t)

	mockContainer := collector.ContainerInfo{
		Name:     "web",
		ImageRef: "nginx:1.24-alpine",
		ImageID:  "sha256:current-id",
		Guard:    domain.GuardAssisted,
		Files:    []string{composePath},
		Service:  "web",
	}

	finder := func(ctx context.Context, target string) (*collector.ContainerInfo, error) {
		return &mockContainer, nil
	}

	orch := NewOrchestrator(nil, stateMgr).WithContainerFinder(finder)
	orch.imagePuller = func(ctx context.Context, ref string) error { return nil }
	orch.imageInspector = func(ctx context.Context, ref string) (string, error) {
		return "sha256:current-id", nil // same ID
	}

	report, err := orch.Update(context.Background(), "web", UpdateOptions{})
	require.NoError(t, err)
	assert.Equal(t, "skipped_identical", report.Status)
	assert.Contains(t, report.Message, "最新状态")
}

func TestOrchestrator_ProbeFailureRollback(t *testing.T) {
	_, composePath, stateMgr := setupTestEnv(t)

	mockContainer := collector.ContainerInfo{
		Name:     "web",
		ImageRef: "nginx:1.24-alpine",
		ImageID:  "sha256:old-id",
		Guard:    domain.GuardAssisted,
		Files:    []string{composePath},
		Service:  "web",
	}

	finder := func(ctx context.Context, target string) (*collector.ContainerInfo, error) {
		return &mockContainer, nil
	}

	taggedBad := false
	composeUpCalls := 0

	orch := NewOrchestrator(nil, stateMgr).WithContainerFinder(finder)
	orch.imagePuller = func(ctx context.Context, ref string) error { return nil }
	orch.imageInspector = func(ctx context.Context, ref string) (string, error) { return "sha256:bad-id", nil }
	orch.imageTagger = func(ctx context.Context, src, target string) error {
		taggedBad = true
		assert.Contains(t, target, "ops-bad-")
		return nil
	}
	orch.composeUpper = func(ctx context.Context, composeFile string, service string) error {
		composeUpCalls++
		return nil
	}
	// First call fails, second call (rollback) succeeds
	firstWait := true
	orch.waiter = func(ctx context.Context, c *collector.ContainerInfo) (bool, string) {
		if firstWait {
			firstWait = false
			return false, "container crash looped"
		}
		return true, "restored container is healthy"
	}

	report, err := orch.Update(context.Background(), "web", UpdateOptions{Tag: "1.25-alpine"})
	require.Error(t, err)
	assert.Equal(t, "rolled_back", report.Status)
	assert.True(t, taggedBad)
	assert.Equal(t, 2, composeUpCalls) // 1 for new version, 1 for rollback

	// Compose file should be restored to original tag
	content, _ := os.ReadFile(composePath)
	assert.Contains(t, string(content), "nginx:1.24-alpine")

	// State history should record rollback
	state, err := stateMgr.Load()
	require.NoError(t, err)
	containerState, exists := state.Containers["web"]
	require.True(t, exists)
	assert.Equal(t, "rolled-back", containerState.Result)
}

func TestOrchestrator_RollbackCommand(t *testing.T) {
	_, composePath, stateMgr := setupTestEnv(t)

	// Pre-record state where current is 1.25 and previous was 1.24
	err := stateMgr.RecordUpdate("web", domain.HistoryEntry{
		At:       time.Now().Format(time.RFC3339),
		Result:   "success",
		ImageRef: "nginx:1.25-alpine",
		PrevRef:  "nginx:1.24-alpine",
		ImageID:  "sha256:id-125",
	})
	require.NoError(t, err)

	// Currently compose file has 1.25
	modifier := compose.NewYamlModifier()
	_, _ = modifier.UpdateImageTag(composePath, "web", "nginx:1.25-alpine")

	mockContainer := collector.ContainerInfo{
		Name:     "web",
		ImageRef: "nginx:1.25-alpine",
		Guard:    domain.GuardAssisted,
		Files:    []string{composePath},
		Service:  "web",
	}

	finder := func(ctx context.Context, target string) (*collector.ContainerInfo, error) {
		return &mockContainer, nil
	}

	orch := NewOrchestrator(nil, stateMgr).WithContainerFinder(finder)
	orch.imagePuller = func(ctx context.Context, ref string) error { return nil }
	orch.imageInspector = func(ctx context.Context, ref string) (string, error) { return "sha256:id-124", nil }
	orch.composeUpper = func(ctx context.Context, composeFile string, service string) error { return nil }
	orch.waiter = func(ctx context.Context, c *collector.ContainerInfo) (bool, string) { return true, "healthy" }

	report, err := orch.Rollback(context.Background(), "web")
	require.NoError(t, err)
	assert.Equal(t, "success", report.Status)
	assert.Equal(t, "nginx:1.24-alpine", report.RestoredRef)

	// Compose file restored
	content, _ := os.ReadFile(composePath)
	assert.Contains(t, string(content), "nginx:1.24-alpine")
}
