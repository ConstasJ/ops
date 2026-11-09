package docker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/docker"
)

func TestNewClient_InvalidHost(t *testing.T) {
	ctx := context.Background()
	_, err := docker.NewClient(ctx, "::invalid-host::")
	assert.Error(t, err)
}

func TestNewClient_MockServer(t *testing.T) {
	// Create mock docker daemon HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_ping", "/v1.56/_ping":
			w.Header().Set("API-Version", "1.56")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/v1.56/version", "/version":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Version":"29.9.2","ApiVersion":"1.56","GoVersion":"go1.26.10","Os":"linux","Arch":"amd64"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	c, err := docker.NewClient(ctx, server.URL)
	require.NoError(t, err)
	defer c.Close()

	// Test Ping
	ping, err := c.Ping(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.56", ping.APIVersion)

	// Test ServerVersion
	ver, err := c.ServerVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, "29.9.2", ver.Version)
	assert.Equal(t, "linux", ver.Os)
	assert.Equal(t, "amd64", ver.Arch)

	// Verify Engine and Compose interfaces are accessible
	assert.NotNil(t, c.Engine())
	assert.NotNil(t, c.Compose())
}
