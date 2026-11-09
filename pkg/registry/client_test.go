package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		expectedRepo string
		expectedTag  string
	}{
		{
			name:         "docker hub official simple",
			ref:          "postgres:16-alpine",
			expectedRepo: "index.docker.io/library/postgres",
			expectedTag:  "16-alpine",
		},
		{
			name:         "docker hub without tag",
			ref:          "postgres",
			expectedRepo: "index.docker.io/library/postgres",
			expectedTag:  "latest",
		},
		{
			name:         "ghcr image with tag",
			ref:          "ghcr.io/owner/repo:v1.2.3",
			expectedRepo: "ghcr.io/owner/repo",
			expectedTag:  "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag, err := ParseImageReference(tt.ref)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRepo, repo)
			assert.Equal(t, tt.expectedTag, tag)
		})
	}
}

func TestListTags_MockRegistry(t *testing.T) {
	// 启动一个 mock Registry V2 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test/app",
				"tags": []string{"1.0.0", "1.1.0", "1.2.0-beta.1", "latest"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	host := u.Host

	client := NewClient(WithInsecure(true), WithAuth(authn.Anonymous))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tags, err := client.ListTags(ctx, host+"/test/app:latest")
	require.NoError(t, err)
	assert.Contains(t, tags, "1.0.0")
	assert.Contains(t, tags, "1.1.0")
	assert.Contains(t, tags, "latest")
}

func TestGetDigest_MockRegistry(t *testing.T) {
	expectedDigest := "sha256:ba5e000000000000000000000000000000000000000000000000000000000001"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Docker-Content-Digest", expectedDigest)
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			// 返回一个极简合法 manifest body
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":100,"digest":"` + expectedDigest + `"},"layers":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	host := u.Host

	client := NewClient(WithInsecure(true), WithAuth(authn.Anonymous))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	digest, err := client.GetDigest(ctx, host+"/test/app:1.0.0")
	require.NoError(t, err)
	assert.Equal(t, expectedDigest, digest)
}
