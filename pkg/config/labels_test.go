package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"ops-go/pkg/domain"
)

func TestParseLabels_AllFields(t *testing.T) {
	labels := map[string]string{
		"ops.guard":          "db",
		"ops.kind":           "postgres",
		"ops.probe.http":     "http://:8080/api/health",
		"ops.probe.tcp":      ":5432",
		"ops.probe.insecure": "true",
		"ops.timeout":        "120s",
		"ops.settle":         "10s",
		"ops.notes":          "生产数据库，更新前自动做逻辑备份",
		"ops.extra_dump":     "pg_dump -Fc mydb > /tmp/mydb.dump",
	}

	spec := ParseLabels(labels)
	assert.NotNil(t, spec)
	assert.Equal(t, domain.GuardDB, spec.Guard)
	assert.Equal(t, "postgres", spec.Kind)
	assert.NotNil(t, spec.Probe)
	assert.Equal(t, "http://:8080/api/health", spec.Probe.HTTP)
	assert.Equal(t, ":5432", spec.Probe.TCP)
	assert.True(t, spec.Probe.Insecure)
	assert.Equal(t, 120*time.Second, spec.GetTimeout())
	assert.Equal(t, 10*time.Second, spec.GetSettle())
	assert.Equal(t, "生产数据库，更新前自动做逻辑备份", spec.Notes)
	assert.Equal(t, "pg_dump -Fc mydb > /tmp/mydb.dump", spec.ExtraDump)
}

func TestParseLabels_EmptyOrNoOps(t *testing.T) {
	labels := map[string]string{
		"com.docker.compose.project": "myproject",
		"unrelated.label":            "value",
	}

	spec := ParseLabels(labels)
	assert.Nil(t, spec)
}

func TestMergeSpecWithLabels(t *testing.T) {
	baseSpec := domain.ContainerSpec{
		Guard: domain.GuardAssisted,
		Kind:  "generic",
		Notes: "全局配置的说明",
	}

	labels := map[string]string{
		"ops.guard":      "db",
		"ops.probe.http": "http://:3000/health",
	}

	merged := MergeSpec(baseSpec, ParseLabels(labels))
	assert.Equal(t, domain.GuardDB, merged.Guard) // label 覆盖了 base
	assert.Equal(t, "generic", merged.Kind)        // 未被 label 指定，保留 base
	assert.Equal(t, "全局配置的说明", merged.Notes)    // 保留 base
	assert.NotNil(t, merged.Probe)
	assert.Equal(t, "http://:3000/health", merged.Probe.HTTP)
}
