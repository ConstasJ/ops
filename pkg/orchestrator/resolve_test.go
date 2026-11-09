package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTargetImage(t *testing.T) {
	assert.Equal(t, "redis:7.2-alpine", ResolveTargetImage("redis:7.0-alpine", "", "redis:7.2-alpine"))
	assert.Equal(t, "redis:7.2-alpine", ResolveTargetImage("redis:7.0-alpine", "7.2-alpine", ""))
	assert.Equal(t, "redis:7.0-alpine", ResolveTargetImage("redis:7.0-alpine", "", ""))
	assert.Equal(t, "docker.io/library/nginx:1.25", ResolveTargetImage("docker.io/library/nginx:1.24", "1.25", ""))
	assert.Equal(t, "myregistry.internal:5000/app:v2", ResolveTargetImage("myregistry.internal:5000/app:v1", "v2", ""))
}
