package collector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"ops-go/pkg/docker"
	"ops-go/pkg/domain"
)

// Collector handles gathering and enriching container metadata
type Collector struct {
	dockerClient *docker.Client
	config       *domain.ContainersConfig
}

// NewCollector constructs a new Collector
func NewCollector(dockerClient *docker.Client, cfg *domain.ContainersConfig) *Collector {
	if cfg == nil {
		cfg = &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}
	}
	return &Collector{
		dockerClient: dockerClient,
		config:       cfg,
	}
}

// ResolveSpec determines the ContainerSpec for a given container
func (col *Collector) ResolveSpec(name string, isAuto bool) domain.ContainerSpec {
	if spec, ok := col.config.FindSpec(name); ok {
		return spec
	}
	if isAuto {
		return domain.ContainerSpec{
			Guard: domain.GuardAuto,
		}
	}
	return domain.ContainerSpec{
		Guard: domain.GuardAssisted,
	}
}

// Collect queries Docker daemon and returns all containers with their operational metadata
func (col *Collector) Collect(ctx context.Context) ([]ContainerInfo, error) {
	if col.dockerClient == nil {
		return nil, fmt.Errorf("docker client is not configured")
	}

	res, err := col.dockerClient.Engine().ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var out []ContainerInfo
	for _, summary := range res.Items {
		name := ""
		if len(summary.Names) > 0 {
			name = strings.TrimPrefix(summary.Names[0], "/")
		}

		inspectRes, err := col.dockerClient.Engine().ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if err != nil {
			// Container might have vanished between list and inspect
			continue
		}
		c := inspectRes.Container

		labels := summary.Labels
		if labels == nil {
			labels = make(map[string]string)
		}

		isAuto := labels["wud.trigger.dockercompose.local.enabled"] == "true"
		spec := col.ResolveSpec(name, isAuto)

		var files []string
		if rawFiles := labels["com.docker.compose.project.config_files"]; rawFiles != "" {
			for _, f := range strings.Split(rawFiles, ",") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					files = append(files, trimmed)
				}
			}
		}

		healthStatus := ""
		if c.State != nil && c.State.Health != nil {
			healthStatus = string(c.State.Health.Status)
		}

		var startedAt time.Time
		if c.State != nil && c.State.StartedAt != "" {
			startedAt, _ = time.Parse(time.RFC3339Nano, c.State.StartedAt)
		}

		restartCount := 0
		if c.RestartCount != 0 {
			restartCount = c.RestartCount
		}

		exitCode := 0
		stateStatus := string(summary.State)
		if c.State != nil {
			stateStatus = string(c.State.Status)
			exitCode = c.State.ExitCode
		}

		info := ContainerInfo{
			ID:           summary.ID,
			Name:         name,
			ImageRef:     summary.Image,
			ImageID:      summary.ImageID,
			State:        stateStatus,
			Health:       healthStatus,
			RestartCount: restartCount,
			ExitCode:     exitCode,
			StartedAt:    startedAt,
			Project:      labels["com.docker.compose.project"],
			Service:      labels["com.docker.compose.service"],
			Files:        files,
			WorkingDir:   labels["com.docker.compose.project.working_dir"],
			Auto:         isAuto,

			Kind:      spec.Kind,
			Guard:     spec.EffectiveGuard(),
			Notes:     spec.Notes,
			Probe:     spec.Probe,
			Timeout:   spec.GetTimeout(),
			Settle:    spec.GetSettle(),
			ExtraDump: spec.ExtraDump,
		}
		out = append(out, info)
	}

	// Sort order: manual containers first (sorted by name), then auto containers (sorted by name)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Auto != out[j].Auto {
			return !out[i].Auto // false (manual) comes first
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}

// Find locates a container by target string (name, project/service, service, or substring)
func (col *Collector) Find(ctx context.Context, target string) (*ContainerInfo, error) {
	containers, err := col.Collect(ctx)
	if err != nil {
		return nil, err
	}
	return FindInList(containers, target)
}

// FindInList searches a pre-collected list of containers
func FindInList(containers []ContainerInfo, target string) (*ContainerInfo, error) {
	targetLower := strings.ToLower(target)

	// 1. Exact name match
	for _, c := range containers {
		if strings.EqualFold(c.Name, target) {
			return &c, nil
		}
	}

	// 2. Exact project/service match (e.g. "linknet/db")
	for _, c := range containers {
		if strings.EqualFold(c.StackService(), target) {
			return &c, nil
		}
	}

	// 3. Exact service match if project is non-empty
	var serviceMatches []ContainerInfo
	for _, c := range containers {
		if c.Service != "" && strings.EqualFold(c.Service, target) {
			serviceMatches = append(serviceMatches, c)
		}
	}
	if len(serviceMatches) == 1 {
		return &serviceMatches[0], nil
	}

	// 4. Substring match on name
	var subMatches []ContainerInfo
	for _, c := range containers {
		if strings.Contains(strings.ToLower(c.Name), targetLower) {
			subMatches = append(subMatches, c)
		}
	}

	if len(subMatches) == 1 {
		return &subMatches[0], nil
	}
	if len(subMatches) > 1 {
		var names []string
		for _, m := range subMatches {
			names = append(names, m.Name)
		}
		return nil, fmt.Errorf("目标 %q 匹配到多个容器: %s", target, strings.Join(names, ", "))
	}

	return nil, fmt.Errorf("找不到容器 %q", target)
}
