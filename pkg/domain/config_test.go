package domain_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/domain"
)

func TestGuardLevel_Validation(t *testing.T) {
	tests := []struct {
		input    string
		expected domain.GuardLevel
		valid    bool
	}{
		{"auto", domain.GuardAuto, true},
		{"assisted", domain.GuardAssisted, true},
		{"db", domain.GuardDB, true},
		{"frozen", domain.GuardFrozen, true},
		{"locked", domain.GuardLocked, true},
		{"invalid", "", false},
		{"", domain.GuardAssisted, true}, // empty string defaults to assisted
	}

	for _, tt := range tests {
		var g domain.GuardLevel
		err := g.UnmarshalText([]byte(tt.input))
		if tt.valid {
			assert.NoError(t, err, "input: %s", tt.input)
			assert.Equal(t, tt.expected, g)
		} else {
			assert.Error(t, err, "input: %s", tt.input)
		}
	}
}

func TestLoadConfig_RealContainersJSON(t *testing.T) {
	// Sample JSON based on real containers.json
	rawJSON := `{
		"_comment": "test knowledge base",
		"linknet-db-1": {
			"kind": "postgres",
			"timeout": 150,
			"notes": "pgvector test note"
		},
		"gitea": {
			"kind": "gitea",
			"timeout": 240,
			"settle": 15,
			"probe": { "http": "http://127.0.0.1:3000/api/healthz" },
			"notes": "gitea test note"
		},
		"nextskin-nextskin-1": {
			"guard": "frozen",
			"probe": { "http": "http://127.0.0.1:3030/", "insecure": false },
			"notes": "frozen note"
		}
	}`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "containers.json")
	err := os.WriteFile(configPath, []byte(rawJSON), 0644)
	require.NoError(t, err)

	cfg, err := domain.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Test linknet-db-1
	spec, found := cfg.FindSpec("linknet-db-1")
	assert.True(t, found)
	assert.Equal(t, "postgres", spec.Kind)
	assert.Equal(t, domain.GuardDB, spec.EffectiveGuard()) // Inferred from kind postgres
	assert.Equal(t, 150*time.Second, spec.GetTimeout())
	assert.Equal(t, 5*time.Second, spec.GetSettle()) // default settle

	// Test gitea
	giteaSpec, found := cfg.FindSpec("gitea")
	assert.True(t, found)
	assert.Equal(t, domain.GuardDB, giteaSpec.EffectiveGuard())
	assert.Equal(t, 15*time.Second, giteaSpec.GetSettle())
	require.NotNil(t, giteaSpec.Probe)
	assert.Equal(t, "http://127.0.0.1:3000/api/healthz", giteaSpec.Probe.HTTP)

	// Test nextskin
	nextskinSpec, found := cfg.FindSpec("nextskin-nextskin-1")
	assert.True(t, found)
	assert.Equal(t, domain.GuardFrozen, nextskinSpec.EffectiveGuard())
}
