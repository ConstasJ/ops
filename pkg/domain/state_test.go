package domain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/domain"
)

func TestStateManager_LoadAndRecord(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := domain.NewStateManager(statePath)

	// File doesn't exist yet -> should return empty state, no error
	state, err := sm.Load()
	require.NoError(t, err)
	assert.Empty(t, state.Containers)

	// Record an update
	record := domain.HistoryEntry{
		At:       "2026-11-09T10:00:00",
		Result:   "success",
		ImageID:  "sha256:1234567890abcdef",
		ImageRef: "postgres:16-alpine",
		PrevRef:  "postgres:15-alpine",
		RefAfter: "postgres:16-alpine",
		Note:     "test update",
	}

	err = sm.RecordUpdate("test-db-1", record)
	require.NoError(t, err)

	// Reload and verify
	state2, err := sm.Load()
	require.NoError(t, err)
	cState, exists := state2.Containers["test-db-1"]
	require.True(t, exists)
	assert.Equal(t, "success", cState.Result)
	assert.Equal(t, "sha256:1234567890abcdef", cState.ImageID)
	assert.Equal(t, "postgres:16-alpine", cState.ImageRef)
	assert.Equal(t, "test update", cState.LastNote)
	require.Len(t, cState.History, 1)
	assert.Equal(t, record, cState.History[0])

	// Record another update -> should append to history and update top-level
	record2 := domain.HistoryEntry{
		At:       "2026-11-09T11:00:00",
		Result:   "rolled-back",
		ImageID:  "sha256:previmageid",
		ImageRef: "postgres:15-alpine",
		Note:     "rollback after failure",
	}
	err = sm.RecordUpdate("test-db-1", record2)
	require.NoError(t, err)

	state3, err := sm.Load()
	require.NoError(t, err)
	cState3 := state3.Containers["test-db-1"]
	assert.Equal(t, "rolled-back", cState3.Result)
	assert.Equal(t, "rollback after failure", cState3.LastNote)
	assert.Len(t, cState3.History, 2)
}

func TestStateManager_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	err := os.WriteFile(statePath, []byte("invalid json"), 0644)
	require.NoError(t, err)

	sm := domain.NewStateManager(statePath)
	_, err = sm.Load()
	assert.Error(t, err)
}
