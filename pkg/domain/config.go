package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ProbeSpec defines HTTP or TCP liveness probe configuration
type ProbeSpec struct {
	HTTP     string `json:"http,omitempty"`
	TCP      string `json:"tcp,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

// ContainerSpec defines rules, protection level, and probes for a container
type ContainerSpec struct {
	Kind      string      `json:"kind,omitempty"`
	Guard     GuardLevel  `json:"guard,omitempty"`
	Timeout   int         `json:"timeout,omitempty"` // in seconds
	Settle    int         `json:"settle,omitempty"`  // in seconds
	Probe     *ProbeSpec  `json:"probe,omitempty"`
	Notes     string      `json:"notes,omitempty"`
	ExtraDump string      `json:"extra_dump,omitempty"`
}

// EffectiveGuard returns the explicit Guard if set, or infers it from Kind
func (s *ContainerSpec) EffectiveGuard() GuardLevel {
	if s.Guard != "" && s.Guard.IsValid() {
		return s.Guard
	}
	switch strings.ToLower(s.Kind) {
	case "postgres", "redis", "gitea", "sqlite", "mariadb", "mysql":
		return GuardDB
	default:
		return GuardAssisted
	}
}

// GetTimeout returns the timeout as time.Duration with a safe default (120s)
func (s *ContainerSpec) GetTimeout() time.Duration {
	if s.Timeout > 0 {
		return time.Duration(s.Timeout) * time.Second
	}
	return 120 * time.Second
}

// GetSettle returns the settle time as time.Duration with a safe default (5s)
func (s *ContainerSpec) GetSettle() time.Duration {
	if s.Settle > 0 {
		return time.Duration(s.Settle) * time.Second
	}
	return 5 * time.Second
}

// ContainersConfig holds the parsed containers.json mapping
type ContainersConfig struct {
	Specs map[string]ContainerSpec
}

// LoadConfig reads and parses containers.json, skipping comment keys
func LoadConfig(path string) (*ContainersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse json %s: %w", path, err)
	}

	specs := make(map[string]ContainerSpec)
	for key, rawVal := range raw {
		if strings.HasPrefix(key, "_") {
			continue // skip _comment or other metadata
		}
		var spec ContainerSpec
		if err := json.Unmarshal(rawVal, &spec); err != nil {
			return nil, fmt.Errorf("failed to parse container %q spec: %w", key, err)
		}
		specs[key] = spec
	}

	return &ContainersConfig{Specs: specs}, nil
}

// FindSpec looks up a spec by container name or pattern
func (c *ContainersConfig) FindSpec(name string) (ContainerSpec, bool) {
	if spec, ok := c.Specs[name]; ok {
		return spec, true
	}
	// Fallback to fuzzy substring or prefix if needed in later phases
	return ContainerSpec{}, false
}
