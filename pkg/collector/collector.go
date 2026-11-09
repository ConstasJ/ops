package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"ops-go/pkg/compose"
	"ops-go/pkg/config"
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

		labels := make(map[string]string)
		for k, v := range summary.Labels {
			labels[k] = v
		}
		if c.Config != nil {
			for k, v := range c.Config.Labels {
				if _, exists := labels[k]; !exists {
					labels[k] = v
				}
			}
		}

		isAuto := labels["wud.trigger.dockercompose.local.enabled"] == "true"
		spec := col.ResolveSpec(name, isAuto)
		if opsLabelSpec := config.ParseLabels(labels); opsLabelSpec != nil {
			spec = config.MergeSpec(spec, opsLabelSpec)
		}

		var files []string
		if rawFiles := labels["com.docker.compose.project.config_files"]; rawFiles != "" {
			for _, f := range strings.Split(rawFiles, ",") {
				if trimmed := strings.TrimSpace(f); trimmed != "" {
					files = append(files, trimmed)
				}
			}
		} else if workdir := labels["com.docker.compose.project.working_dir"]; workdir != "" {
			for _, candidate := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
				candidatePath := filepath.Join(workdir, candidate)
				files = append(files, candidatePath)
				break
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

		imageRef := summary.Image
		if c.Config != nil && c.Config.Image != "" {
			imageRef = c.Config.Image
		}
		imageID := summary.ImageID
		if c.Image != "" {
			imageID = c.Image
		}

		declaredImage := ""
		hasDrift := false
		serviceName := labels["com.docker.compose.service"]
		workingDir := labels["com.docker.compose.project.working_dir"]
		if len(files) > 0 && serviceName != "" {
			if declared, err := compose.GetDeclaredImage(files, workingDir, serviceName); err == nil && declared != "" {
				declaredImage = declared
				if !isSameImageRef(imageRef, declaredImage) {
					hasDrift = true
				}
			}
		}

		info := ContainerInfo{
			ID:            summary.ID,
			Name:          name,
			ImageRef:      imageRef,
			ImageID:       imageID,
			DeclaredImage: declaredImage,
			HasDrift:      hasDrift,
			State:         stateStatus,
			Health:        healthStatus,
			RestartCount:  restartCount,
			ExitCode:      exitCode,
			StartedAt:     startedAt,
			Project:       labels["com.docker.compose.project"],
			Service:       serviceName,
			Files:         files,
			WorkingDir:    workingDir,
			Auto:          isAuto,

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

// ResolveTargets resolves multiple targets (names, project/service, service, or project/stack)
func (col *Collector) ResolveTargets(ctx context.Context, targets []string) ([]ContainerInfo, error) {
	containers, err := col.Collect(ctx)
	if err != nil {
		return nil, err
	}
	return ResolveTargetsInList(containers, targets)
}

// ResolveTargetsInList resolves target strings against a slice of ContainerInfo
func ResolveTargetsInList(containers []ContainerInfo, targets []string) ([]ContainerInfo, error) {
	var results []ContainerInfo
	seen := make(map[string]bool)

	for _, rawTarget := range targets {
		target := strings.TrimSpace(rawTarget)
		if target == "" {
			continue
		}

		var matched []ContainerInfo

		// 1. Exact name match
		for _, c := range containers {
			if strings.EqualFold(c.Name, target) {
				matched = append(matched, c)
			}
		}

		// 2. Exact project/service match (e.g. "linknet/db")
		if len(matched) == 0 {
			for _, c := range containers {
				if strings.EqualFold(c.StackService(), target) {
					matched = append(matched, c)
				}
			}
		}

		// 3. Project / Stack name match (e.g. "optest2", "linknet")
		if len(matched) == 0 {
			for _, c := range containers {
				if strings.EqualFold(c.Project, target) {
					matched = append(matched, c)
				}
			}
		}

		// 4. Exact service match if project is non-empty
		if len(matched) == 0 {
			for _, c := range containers {
				if c.Service != "" && strings.EqualFold(c.Service, target) {
					matched = append(matched, c)
				}
			}
			if len(matched) > 1 {
				var names []string
				for _, m := range matched {
					names = append(names, m.Name)
				}
				return nil, fmt.Errorf("服务名 %q 在多个项目中存在，请使用 project/service 明确指定: %s", target, strings.Join(names, ", "))
			}
		}

		// 5. Substring match on name
		if len(matched) == 0 {
			targetLower := strings.ToLower(target)
			for _, c := range containers {
				if strings.Contains(strings.ToLower(c.Name), targetLower) {
					matched = append(matched, c)
				}
			}
			if len(matched) > 1 {
				var names []string
				for _, m := range matched {
					names = append(names, m.Name)
				}
				return nil, fmt.Errorf("目标 %q 匹配到多个容器: %s", target, strings.Join(names, ", "))
			}
		}

		if len(matched) == 0 {
			return nil, fmt.Errorf("找不到容器或项目 %q", target)
		}

		for _, m := range matched {
			key := m.ID
			if key == "" {
				key = m.Name
			}
			if !seen[key] {
				seen[key] = true
				results = append(results, m)
			}
		}
	}

	return results, nil
}

// FindInList searches a pre-collected list of containers
func FindInList(containers []ContainerInfo, target string) (*ContainerInfo, error) {
	resolved, err := ResolveTargetsInList(containers, []string{target})
	if err != nil {
		return nil, err
	}
	if len(resolved) == 1 {
		return &resolved[0], nil
	}
	if len(resolved) > 1 {
		var names []string
		for _, m := range resolved {
			names = append(names, m.Name)
		}
		return nil, fmt.Errorf("目标 %q 匹配到多个容器: %s", target, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("找不到容器 %q", target)
}

func isSameImageRef(a, b string) bool {
	if a == b {
		return true
	}
	normA := normalizeImageRef(a)
	normB := normalizeImageRef(b)
	return normA == normB
}

func normalizeImageRef(ref string) string {
	ref = strings.TrimPrefix(ref, "docker.io/library/")
	ref = strings.TrimPrefix(ref, "docker.io/")
	// 如果不带 tag 且不带 digest，规范化加上 :latest
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}
	return ref
}

