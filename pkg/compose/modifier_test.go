package compose

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleComposeYAML = `# Top-level comment
version: "3.8"

services:
  web:
    # Service comment
    image: nginx:1.24-alpine
    restart: unless-stopped
    ports:
      - "80:80"

  db:
    image: postgres:15-alpine # Inline comment
    environment:
      POSTGRES_DB: app
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:
`

func TestYamlModifier_UpdateImageTag(t *testing.T) {
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	err := os.WriteFile(composePath, []byte(sampleComposeYAML), 0644)
	require.NoError(t, err)

	modifier := NewYamlModifier()

	t.Run("Successfully update target service image tag while preserving comments and indent", func(t *testing.T) {
		res, err := modifier.UpdateImageTag(composePath, "db", "postgres:16-alpine")
		require.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, "postgres:15-alpine", res.OriginalImage)
		assert.Equal(t, "postgres:16-alpine", res.NewImage)
		assert.FileExists(t, res.BackupPath)
		assert.Equal(t, composePath, res.ModifiedFile)

		// Read modified content
		content, err := os.ReadFile(composePath)
		require.NoError(t, err)
		contentStr := string(content)

		// Verify target changed
		assert.Contains(t, contentStr, "image: postgres:16-alpine")
		// Verify inline comment preserved
		assert.Contains(t, contentStr, "# Inline comment")
		// Verify sibling service untouched
		assert.Contains(t, contentStr, "image: nginx:1.24-alpine")
		// Verify comments untouched
		assert.Contains(t, contentStr, "# Top-level comment")
		assert.Contains(t, contentStr, "# Service comment")
	})

	t.Run("Non-existent service returns error and does not modify file", func(t *testing.T) {
		beforeContent, err := os.ReadFile(composePath)
		require.NoError(t, err)

		_, err = modifier.UpdateImageTag(composePath, "nonexistent", "ubuntu:latest")
		require.Error(t, err)

		afterContent, err := os.ReadFile(composePath)
		require.NoError(t, err)
		assert.Equal(t, string(beforeContent), string(afterContent))
	})

	t.Run("RestoreBackup restores original file content and mod time", func(t *testing.T) {
		// Update web service
		res, err := modifier.UpdateImageTag(composePath, "web", "nginx:1.25-alpine")
		require.NoError(t, err)

		updatedContent, err := os.ReadFile(composePath)
		require.NoError(t, err)
		assert.Contains(t, string(updatedContent), "nginx:1.25-alpine")

		time.Sleep(10 * time.Millisecond)

		// Restore backup
		err = modifier.RestoreBackup(res.BackupPath, composePath)
		require.NoError(t, err)

		restoredContent, err := os.ReadFile(composePath)
		require.NoError(t, err)
		assert.Contains(t, string(restoredContent), "nginx:1.24-alpine")
	})

	t.Run("Handles multiple services with similar names", func(t *testing.T) {
		customYAML := `services:
  app:
    image: myapp:1.0
  app-worker:
    image: myapp-worker:1.0
`
		customFile := filepath.Join(tmpDir, "custom-compose.yml")
		require.NoError(t, os.WriteFile(customFile, []byte(customYAML), 0644))

		res, err := modifier.UpdateImageTag(customFile, "app", "myapp:2.0")
		require.NoError(t, err)
		assert.Equal(t, "myapp:1.0", res.OriginalImage)

		data, err := os.ReadFile(customFile)
		require.NoError(t, err)
		dataStr := string(data)
		assert.Contains(t, dataStr, "image: myapp:2.0")
		assert.Contains(t, dataStr, "image: myapp-worker:1.0")
	})

	t.Run("Preserves double quotes around image", func(t *testing.T) {
		customYAML := `services:
  app:
    image: "myapp:1.0"
`
		customFile := filepath.Join(tmpDir, "quoted-compose.yml")
		require.NoError(t, os.WriteFile(customFile, []byte(customYAML), 0644))

		res, err := modifier.UpdateImageTag(customFile, "app", "myapp:2.0")
		require.NoError(t, err)
		assert.Equal(t, "myapp:1.0", res.OriginalImage)

		data, err := os.ReadFile(customFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), `image: "myapp:2.0"`)
	})

	t.Run("Succeeds when other services have required but missing environment variables", func(t *testing.T) {
		envRequiredYAML := `services:
  redis:
    image: redis:8-alpine3.23
  app:
    image: myapp:1.0
    environment:
      SECRET: ${SECRET_KEY_MISSING:?must be set}
`
		envFile := filepath.Join(tmpDir, "missing-env.yml")
		require.NoError(t, os.WriteFile(envFile, []byte(envRequiredYAML), 0644))

		res, err := modifier.UpdateImageTag(envFile, "redis", "redis:8-alpine")
		require.NoError(t, err)
		assert.Equal(t, "redis:8-alpine3.23", res.OriginalImage)

		data, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "image: redis:8-alpine")
		assert.Contains(t, string(data), "${SECRET_KEY_MISSING:?must be set}")
	})

	t.Run("Successfully replaces folded block scalar images", func(t *testing.T) {
		foldedYAML := `services:
  web:
    image: >-
      ghcr.io/org/repo:${TAG:?must
      be set}
    ports:
      - "3000:3000"
`
		foldedFile := filepath.Join(tmpDir, "folded-compose.yml")
		require.NoError(t, os.WriteFile(foldedFile, []byte(foldedYAML), 0644))

		res, err := modifier.UpdateImageTag(foldedFile, "web", "ghcr.io/org/repo:v1.2.3")
		require.NoError(t, err)
		assert.Equal(t, ">-", res.OriginalImage)

		data, err := os.ReadFile(foldedFile)
		require.NoError(t, err)
		dataStr := string(data)
		assert.Contains(t, dataStr, "image: ghcr.io/org/repo:v1.2.3")
		assert.NotContains(t, dataStr, ">-")
		assert.NotContains(t, dataStr, "be set}")
		assert.Contains(t, dataStr, `- "3000:3000"`)
	})
}
