package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateUpdate(t *testing.T) {
	tests := []struct {
		name          string
		currentRef    string
		remoteTags    []string
		currentDigest string
		remoteDigest  string
		expected      UpdateCandidate
	}{
		{
			name:          "patch update",
			currentRef:    "postgres:16.1-alpine",
			remoteTags:    []string{"16.0-alpine", "16.1-alpine", "16.2-alpine", "16.3-alpine", "17.0-alpine"},
			currentDigest: "sha256:111",
			remoteDigest:  "sha256:222",
			expected: UpdateCandidate{
				HasUpdate:  true,
				CurrentTag: "16.1-alpine",
				TargetTag:  "16.3-alpine", // 同 major 下最新 patch/minor
				UpdateType: UpdateMinor,  // 16.3 vs 16.1
			},
		},
		{
			name:          "pure semver patch update",
			currentRef:    "myorg/app:1.2.3",
			remoteTags:    []string{"1.2.3", "1.2.4", "1.2.5-rc.1"},
			currentDigest: "sha256:111",
			remoteDigest:  "sha256:111",
			expected: UpdateCandidate{
				HasUpdate:  true,
				CurrentTag: "1.2.3",
				TargetTag:  "1.2.4",
				UpdateType: UpdatePatch,
			},
		},
		{
			name:          "ignore prerelease when current is stable",
			currentRef:    "myorg/app:v2.0.0",
			remoteTags:    []string{"v2.0.0", "v2.1.0-alpha.1", "v2.0.1-beta.2"},
			currentDigest: "sha256:111",
			remoteDigest:  "sha256:111",
			expected: UpdateCandidate{
				HasUpdate:  false,
				CurrentTag: "v2.0.0",
				TargetTag:  "v2.0.0",
				UpdateType: UpdateNone,
			},
		},
		{
			name:          "latest tag with new digest",
			currentRef:    "redis:latest",
			remoteTags:    []string{"latest", "7.2.4"},
			currentDigest: "sha256:old_digest",
			remoteDigest:  "sha256:new_digest",
			expected: UpdateCandidate{
				HasUpdate:     true,
				CurrentTag:    "latest",
				TargetTag:     "latest",
				UpdateType:    UpdateDigest,
				CurrentDigest: "sha256:old_digest",
				RemoteDigest:  "sha256:new_digest",
			},
		},
		{
			name:          "already up to date with same digest",
			currentRef:    "nginx:1.25.4-alpine",
			remoteTags:    []string{"1.25.3-alpine", "1.25.4-alpine"},
			currentDigest: "sha256:same",
			remoteDigest:  "sha256:same",
			expected: UpdateCandidate{
				HasUpdate:     false,
				CurrentTag:    "1.25.4-alpine",
				TargetTag:     "1.25.4-alpine",
				UpdateType:    UpdateNone,
				CurrentDigest: "sha256:same",
				RemoteDigest:  "sha256:same",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := EvaluateUpdate(tt.currentRef, tt.remoteTags, tt.currentDigest, tt.remoteDigest)
			assert.Equal(t, tt.expected.HasUpdate, res.HasUpdate)
			assert.Equal(t, tt.expected.CurrentTag, res.CurrentTag)
			assert.Equal(t, tt.expected.TargetTag, res.TargetTag)
			assert.Equal(t, tt.expected.UpdateType, res.UpdateType)
			if tt.expected.CurrentDigest != "" {
				assert.Equal(t, tt.expected.CurrentDigest, res.CurrentDigest)
			}
			if tt.expected.RemoteDigest != "" {
				assert.Equal(t, tt.expected.RemoteDigest, res.RemoteDigest)
			}
		})
	}
}

func TestCompareImageRefs(t *testing.T) {
	c1 := CompareImageRefs("gitea/gitea:1.22.0", "gitea/gitea:1.22.1")
	assert.True(t, c1.HasUpdate)
	assert.Equal(t, UpdatePatch, c1.UpdateType)
	assert.Equal(t, "1.22.0", c1.CurrentTag)
	assert.Equal(t, "1.22.1", c1.TargetTag)

	c2 := CompareImageRefs("redis:7.0-alpine", "redis:7.2-alpine")
	assert.True(t, c2.HasUpdate)
	assert.Equal(t, UpdateMinor, c2.UpdateType)

	c3 := CompareImageRefs("postgres:15", "postgres:16")
	assert.True(t, c3.HasUpdate)
	assert.Equal(t, UpdateMajor, c3.UpdateType)

	c4 := CompareImageRefs("myimage:abc", "myimage:xyz")
	assert.True(t, c4.HasUpdate)
	assert.Equal(t, UpdateCompose, c4.UpdateType)
}
