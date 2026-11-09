package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockClient struct {
	mock.Mock
}

func (m *mockClient) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	args := m.Called(ctx, imageRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockClient) GetDigest(ctx context.Context, imageRef string) (string, error) {
	args := m.Called(ctx, imageRef)
	return args.String(0), args.Error(1)
}

func TestCheckUpdate_Success(t *testing.T) {
	m := new(mockClient)
	ctx := context.Background()

	m.On("ListTags", ctx, "library/redis:7.2.3").Return([]string{"7.2.3", "7.2.4", "7.2.5"}, nil)
	m.On("GetDigest", ctx, "library/redis:7.2.3").Return("sha256:remote111", nil)

	cand, err := CheckUpdate(ctx, m, "library/redis:7.2.3", "sha256:local111")
	assert.NoError(t, err)
	assert.True(t, cand.HasUpdate)
	assert.Equal(t, "7.2.5", cand.TargetTag)
	assert.Equal(t, UpdatePatch, cand.UpdateType)
}

func TestCheckUpdate_ListError(t *testing.T) {
	m := new(mockClient)
	ctx := context.Background()

	m.On("ListTags", ctx, "private.hub/app:1.0").Return(nil, errors.New("unauthorized"))

	_, err := CheckUpdate(ctx, m, "private.hub/app:1.0", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}
