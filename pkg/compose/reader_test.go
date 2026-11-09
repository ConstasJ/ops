package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDeclaredImage(t *testing.T) {
	tmpDir := t.TempDir()

	composeContent := `
version: '3.8'
services:
  web:
    image: "nginx:1.25.0-alpine"
    ports:
      - "80:80"
  db:
    image: postgres:16
`
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	err := os.WriteFile(composePath, []byte(composeContent), 0644)
	require.NoError(t, err)

	// Test 1: Web service
	img, err := GetDeclaredImage([]string{composePath}, tmpDir, "web")
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.25.0-alpine", img)

	// Test 2: DB service
	img, err = GetDeclaredImage([]string{composePath}, tmpDir, "db")
	require.NoError(t, err)
	assert.Equal(t, "postgres:16", img)

	// Test 3: Override file precedence
	overrideContent := `
version: '3.8'
services:
  web:
    image: nginx:1.26.0-alpine
`
	overridePath := filepath.Join(tmpDir, "docker-compose.override.yml")
	err = os.WriteFile(overridePath, []byte(overrideContent), 0644)
	require.NoError(t, err)

	img, err = GetDeclaredImage([]string{composePath, overridePath}, tmpDir, "web")
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.26.0-alpine", img)

	// Test 4: Non-existent service
	img, err = GetDeclaredImage([]string{composePath}, tmpDir, "redis")
	require.NoError(t, err)
	assert.Empty(t, img)

	// Test 5: Folded scalar with required but missing variable returns empty (not >- or broken string)
	dynamicContent := `
services:
  app:
    image: >-
      ghcr.io/org/repo:${MISSING_TAG:?missing}
  redis:
    image: redis:8-alpine
`
	dynamicPath := filepath.Join(tmpDir, "dynamic-compose.yml")
	err = os.WriteFile(dynamicPath, []byte(dynamicContent), 0644)
	require.NoError(t, err)

	img, err = GetDeclaredImage([]string{dynamicPath}, tmpDir, "app")
	require.NoError(t, err)
	assert.Empty(t, img, "should not return >- or unresolved variable expression")

	img, err = GetDeclaredImage([]string{dynamicPath}, tmpDir, "redis")
	require.NoError(t, err)
	assert.Equal(t, "redis:8-alpine", img)
}
