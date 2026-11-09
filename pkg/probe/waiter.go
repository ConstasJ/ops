package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// ContainerInspector defines the minimal Docker client interface needed for health inspection
type ContainerInspector interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

// Waiter manages container healthcheck polling and settle window observation
type Waiter struct {
	inspector ContainerInspector
	interval  time.Duration
}

// NewWaiter constructs a new Waiter instance
func NewWaiter(inspector ContainerInspector) *Waiter {
	return &Waiter{
		inspector: inspector,
		interval:  500 * time.Millisecond,
	}
}

// WithInterval sets the polling interval for tests or custom environments
func (w *Waiter) WithInterval(interval time.Duration) *Waiter {
	w.interval = interval
	return w
}

// Wait blocks until container reaches healthy state and remains stable during settle period
func (w *Waiter) Wait(ctx context.Context, c *collector.ContainerInfo) (bool, string) {
	if c == nil {
		return false, "container info is nil"
	}
	if w.inspector == nil {
		return false, "no docker inspector provided"
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	settle := c.Settle
	if settle <= 0 {
		settle = 5 * time.Second
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	targetName := c.Name
	if targetName == "" {
		targetName = c.ID
	}

	// Phase 1: Wait until initial healthy condition is met
	var initialRestartCount int
	firstSuccess := false

	for !firstSuccess {
		select {
		case <-ctxTimeout.Done():
			return false, fmt.Sprintf("timeout waiting for container %s to become healthy", targetName)
		case <-ticker.C:
			res, err := w.inspector.ContainerInspect(ctxTimeout, targetName, client.ContainerInspectOptions{})
			if err != nil {
				continue
			}

			if res.Container.State == nil {
				continue
			}

			state := res.Container.State
			if state.Status == "exited" || state.Status == "dead" {
				return false, fmt.Sprintf("container %s exited with code %d", targetName, state.ExitCode)
			}

			if state.Status != "running" {
				continue
			}

			initialRestartCount = res.Container.RestartCount

			if state.Health != nil {
				if state.Health.Status == container.Healthy {
					firstSuccess = true
				}
			} else if c.Probe != nil && (c.Probe.HTTP != "" || c.Probe.TCP != "") {
				ok, _ := CheckProbe(ctxTimeout, c.Probe)
				if ok {
					firstSuccess = true
				}
			} else {
				// No healthcheck and no custom probe: running status is sufficient to enter settle
				firstSuccess = true
			}
		}
	}

	// Phase 2: Settle observation period
	settleTimer := time.NewTimer(settle)
	defer settleTimer.Stop()

	for {
		select {
		case <-settleTimer.C:
			return true, fmt.Sprintf("container %s is healthy and settled (%v)", targetName, settle)
		case <-ctxTimeout.Done():
			return false, fmt.Sprintf("timeout during settle period for container %s", targetName)
		case <-ticker.C:
			res, err := w.inspector.ContainerInspect(ctxTimeout, targetName, client.ContainerInspectOptions{})
			if err != nil {
				return false, fmt.Sprintf("container %s inspect failed during settle: %v", targetName, err)
			}

			if res.Container.State == nil {
				return false, fmt.Sprintf("container %s state missing during settle", targetName)
			}

			state := res.Container.State
			if state.Status != "running" {
				return false, fmt.Sprintf("container %s stopped during settle (status: %s, code: %d)", targetName, state.Status, state.ExitCode)
			}

			if res.Container.RestartCount > initialRestartCount {
				return false, fmt.Sprintf("container %s restarted during settle (restarts: %d -> %d)", targetName, initialRestartCount, res.Container.RestartCount)
			}

			if state.Health != nil && state.Health.Status == container.Unhealthy {
				return false, fmt.Sprintf("container %s became unhealthy during settle", targetName)
			}

			if c.Probe != nil && (c.Probe.HTTP != "" || c.Probe.TCP != "") {
				ok, msg := CheckProbe(ctxTimeout, c.Probe)
				if !ok {
					return false, fmt.Sprintf("container %s probe failed during settle: %s", targetName, msg)
				}
			}
		}
	}
}

// WaitForHealthy checks container health and waits for settle period
func WaitForHealthy(ctx context.Context, cli *docker.Client, c *collector.ContainerInfo) (bool, string) {
	var inspector ContainerInspector
	if cli != nil {
		inspector = cli.Engine()
	}
	return NewWaiter(inspector).Wait(ctx, c)
}
