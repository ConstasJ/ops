package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// HistoryEntry records a single update or rollback attempt
type HistoryEntry struct {
	At         string `json:"at"`
	Result     string `json:"result"` // e.g. "success", "rolled-back", "failed"
	ImageID    string `json:"image_id"`
	ImageRef   string `json:"image_ref,omitempty"`
	PrevRef    string `json:"prev_ref,omitempty"`
	RefAfter   string `json:"ref_after,omitempty"`
	Note       string `json:"note,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
}

// ContainerState holds the current status and update history for a single container
type ContainerState struct {
	History   []HistoryEntry `json:"history"`
	Result    string         `json:"result"`
	ImageID   string         `json:"image_id"`
	ImageRef  string         `json:"image_ref,omitempty"`
	LastNote  string         `json:"last_note,omitempty"`
}

// State represents the root object of state.json
type State struct {
	Containers map[string]ContainerState `json:"containers"`
}

// StateManager provides concurrency-safe reading and writing of state.json
type StateManager struct {
	path string
	mu   sync.RWMutex
}

// NewStateManager creates a new StateManager
func NewStateManager(path string) *StateManager {
	return &StateManager{path: path}
}

// Load reads and parses state.json. Returns an empty State if the file doesn't exist.
func (m *StateManager) Load() (*State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Containers: make(map[string]ContainerState)}, nil
		}
		return nil, fmt.Errorf("failed to read state file %s: %w", m.path, err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("corrupted state file %s: %w", m.path, err)
	}
	if state.Containers == nil {
		state.Containers = make(map[string]ContainerState)
	}
	return &state, nil
}

// Save persists the State to disk atomically using a temp file
func (m *StateManager) Save(state *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for state: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp", m.path)
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tmpFile, m.path); err != nil {
		return fmt.Errorf("failed to rename temp state file to %s: %w", m.path, err)
	}
	return nil
}

// RecordUpdate updates the container state and appends the entry to its history
func (m *StateManager) RecordUpdate(containerName string, entry HistoryEntry) error {
	state, err := m.Load()
	if err != nil {
		return err
	}

	cState, exists := state.Containers[containerName]
	if !exists {
		cState = ContainerState{
			History: make([]HistoryEntry, 0, 1),
		}
	}

	cState.History = append(cState.History, entry)
	cState.Result = entry.Result
	cState.ImageID = entry.ImageID
	cState.ImageRef = entry.ImageRef
	cState.LastNote = entry.Note

	state.Containers[containerName] = cState
	return m.Save(state)
}
