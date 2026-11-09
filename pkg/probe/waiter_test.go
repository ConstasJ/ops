package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/domain"

	"github.com/stretchr/testify/assert"
)

type mockInspector struct {
	inspectFunc func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

func (m *mockInspector) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, containerID, options)
	}
	return client.ContainerInspectResult{}, fmt.Errorf("not implemented")
}

func TestWaiter_WaitForHealthy(t *testing.T) {
	t.Run("Container becomes healthy and passes settle period", func(t *testing.T) {
		callCount := 0
		mock := &mockInspector{
			inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				callCount++
				return client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							Status: "running",
							Health: &container.Health{
								Status: container.Healthy,
							},
						},
						RestartCount: 0,
					},
				}, nil
			},
		}

		waiter := NewWaiter(mock).WithInterval(10 * time.Millisecond)
		info := &collector.ContainerInfo{
			Name:    "test-container",
			Timeout: 1 * time.Second,
			Settle:  50 * time.Millisecond,
		}

		ok, msg := waiter.Wait(context.Background(), info)
		assert.True(t, ok)
		assert.Contains(t, msg, "healthy")
		assert.GreaterOrEqual(t, callCount, 2)
	})

	t.Run("Container exits during wait", func(t *testing.T) {
		mock := &mockInspector{
			inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							Status:   "exited",
							ExitCode: 137,
						},
					},
				}, nil
			},
		}

		waiter := NewWaiter(mock).WithInterval(10 * time.Millisecond)
		info := &collector.ContainerInfo{
			Name:    "test-container",
			Timeout: 500 * time.Millisecond,
		}

		ok, msg := waiter.Wait(context.Background(), info)
		assert.False(t, ok)
		assert.Contains(t, msg, "exited with code 137")
	})

	t.Run("Container restarts during settle period", func(t *testing.T) {
		inspectCount := 0
		mock := &mockInspector{
			inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				inspectCount++
				restartCount := 0
				if inspectCount > 2 {
					restartCount = 1 // restarted during settle
				}
				return client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							Status: "running",
							Health: &container.Health{
								Status: container.Healthy,
							},
						},
						RestartCount: restartCount,
					},
				}, nil
			},
		}

		waiter := NewWaiter(mock).WithInterval(10 * time.Millisecond)
		info := &collector.ContainerInfo{
			Name:    "test-container",
			Timeout: 1 * time.Second,
			Settle:  80 * time.Millisecond,
		}

		ok, msg := waiter.Wait(context.Background(), info)
		assert.False(t, ok)
		assert.Contains(t, msg, "restarted")
	})

	t.Run("HTTP probe fallback when container has no native healthcheck", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		mock := &mockInspector{
			inspectFunc: func(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							Status: "running",
							// No Health field
						},
						RestartCount: 0,
					},
				}, nil
			},
		}

		waiter := NewWaiter(mock).WithInterval(10 * time.Millisecond)
		info := &collector.ContainerInfo{
			Name:    "test-container",
			Probe:   &domain.ProbeSpec{HTTP: ts.URL},
			Timeout: 500 * time.Millisecond,
			Settle:  30 * time.Millisecond,
		}

		ok, msg := waiter.Wait(context.Background(), info)
		assert.True(t, ok)
		assert.Contains(t, msg, "healthy")
	})
}
