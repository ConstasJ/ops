package collector_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/collector"
	"ops-go/pkg/domain"
)

func TestContainerInfo_StackServiceAndFormat(t *testing.T) {
	c := collector.ContainerInfo{
		Name:    "linknet-db-1",
		Project: "linknet",
		Service: "db",
		State:   "running",
		Health:  "healthy",
	}

	assert.Equal(t, "linknet/db", c.StackService())
	assert.True(t, c.IsHealthy())

	bare := collector.ContainerInfo{
		Name:  "socks5",
		State: "running",
	}
	assert.Equal(t, "socks5", bare.StackService())
	assert.True(t, bare.IsHealthy())
}

func TestCollector_ResolveSpec(t *testing.T) {
	cfg := &domain.ContainersConfig{
		Specs: map[string]domain.ContainerSpec{
			"linknet-db-1": {
				Kind:    "postgres",
				Timeout: 150,
				Notes:   "pgvector notes",
			},
			"nextskin-nextskin-1": {
				Guard: domain.GuardFrozen,
				Notes: "frozen schema",
			},
		},
	}

	col := collector.NewCollector(nil, cfg)

	// Test direct match
	spec := col.ResolveSpec("linknet-db-1", false)
	assert.Equal(t, "postgres", spec.Kind)
	assert.Equal(t, domain.GuardDB, spec.EffectiveGuard())
	assert.Equal(t, 150*time.Second, spec.GetTimeout())

	// Test frozen
	frozenSpec := col.ResolveSpec("nextskin-nextskin-1", false)
	assert.Equal(t, domain.GuardFrozen, frozenSpec.EffectiveGuard())

	// Test auto container (WUD) without explicit spec
	autoSpec := col.ResolveSpec("openlist", true)
	assert.Equal(t, domain.GuardAuto, autoSpec.EffectiveGuard())

	// Test standard unconfigured container
	plainSpec := col.ResolveSpec("unknown-app-1", false)
	assert.Equal(t, domain.GuardAssisted, plainSpec.EffectiveGuard())
}

func TestCollector_FindCandidate(t *testing.T) {
	containers := []collector.ContainerInfo{
		{Name: "linknet-db-1", Project: "linknet", Service: "db"},
		{Name: "linknet-redis-1", Project: "linknet", Service: "redis"},
		{Name: "gitea", Project: "gitea", Service: "gitea"},
		{Name: "socks5"},
	}

	// 1. Exact name match
	res, err := collector.FindInList(containers, "linknet-db-1")
	require.NoError(t, err)
	assert.Equal(t, "linknet-db-1", res.Name)

	// 2. Project/Service match
	res, err = collector.FindInList(containers, "linknet/redis")
	require.NoError(t, err)
	assert.Equal(t, "linknet-redis-1", res.Name)

	// 3. Substring match
	res, err = collector.FindInList(containers, "git")
	require.NoError(t, err)
	assert.Equal(t, "gitea", res.Name)

	// 4. Ambiguous substring match
	_, err = collector.FindInList(containers, "linknet")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "匹配到多个容器")

	// 5. Not found
	_, err = collector.FindInList(containers, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "找不到容器")
}
