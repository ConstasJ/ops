package orchestrator

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/client"

	"ops-go/pkg/collector"
	"ops-go/pkg/compose"
	"ops-go/pkg/docker"
	"ops-go/pkg/domain"
	"ops-go/pkg/dumper"
	"ops-go/pkg/probe"
)

// UpdateOptions defines options for the update command
type UpdateOptions struct {
	Tag    string
	Image  string
	Force  bool
	DryRun bool
}

// UpdateReport contains the execution outcome of an update
type UpdateReport struct {
	Target      string        `json:"target"`
	Container   string        `json:"container"`
	Project     string        `json:"project"`
	Service     string        `json:"service"`
	OldImageRef string        `json:"old_image_ref"`
	NewImageRef string        `json:"new_image_ref"`
	OldImageID  string        `json:"old_image_id"`
	NewImageID  string        `json:"new_image_id"`
	BackupPath  string        `json:"backup_path,omitempty"`
	DumpPath    string        `json:"dump_path,omitempty"`
	Status      string        `json:"status"` // "success", "skipped_identical", "rolled_back", "refused", "dump_failed", "failed", "dry_run"
	Message     string        `json:"message"`
	Duration    time.Duration `json:"duration"`
}

// Orchestrator coordinates controlled updates and rollbacks
type Orchestrator struct {
	dockerClient *docker.Client
	stateManager *domain.StateManager
	yamlModifier *compose.YamlModifier
	backupBase   string

	finder         func(ctx context.Context, target string) (*collector.ContainerInfo, error)
	imagePuller    func(ctx context.Context, ref string) error
	imageInspector func(ctx context.Context, ref string) (string, error)
	imageTagger    func(ctx context.Context, src, target string) error
	composeUpper   func(ctx context.Context, composeFile string, service string) error
	waiter         func(ctx context.Context, c *collector.ContainerInfo) (bool, string)
	dumper         dumper.Dumper
}

// NewOrchestrator creates a new Orchestrator instance
func NewOrchestrator(dockerClient *docker.Client, stateMgr *domain.StateManager) *Orchestrator {
	if stateMgr == nil {
		stateMgr = domain.NewStateManager("/var/lib/ops/state.json")
	}
	return &Orchestrator{
		dockerClient: dockerClient,
		stateManager: stateMgr,
		yamlModifier: compose.NewYamlModifier(),
		backupBase:   "/var/backups/ops",
	}
}

// WithContainerFinder overrides container discovery (for testing)
func (o *Orchestrator) WithContainerFinder(f func(ctx context.Context, target string) (*collector.ContainerInfo, error)) *Orchestrator {
	o.finder = f
	return o
}

// WithDumper overrides the dumper (for testing)
func (o *Orchestrator) WithDumper(d dumper.Dumper) *Orchestrator {
	o.dumper = d
	return o
}

// ResolveTargetImage determines the target image reference based on options
func ResolveTargetImage(currentRef, optTag, optImage string) string {
	if optImage != "" {
		return optImage
	}
	if optTag == "" {
		return currentRef
	}
	lastColon := strings.LastIndex(currentRef, ":")
	lastSlash := strings.LastIndex(currentRef, "/")
	if lastColon > lastSlash {
		return currentRef[:lastColon] + ":" + optTag
	}
	return currentRef + ":" + optTag
}

// Update executes the controlled update workflow for a target container
func (o *Orchestrator) Update(ctx context.Context, target string, opts UpdateOptions) (*UpdateReport, error) {
	// 1. Find target container
	var c *collector.ContainerInfo
	var err error
	if o.finder != nil {
		c, err = o.finder(ctx, target)
	} else {
		cfg, _ := domain.LoadConfig("/etc/ops/containers.json")
		col := collector.NewCollector(o.dockerClient, cfg)
		c, err = col.Find(ctx, target)
	}
	if err != nil {
		return &UpdateReport{
			Target:  target,
			Status:  "failed",
			Message: err.Error(),
		}, err
	}
	return o.UpdateContainer(ctx, c, opts)
}

// UpdateContainer executes the controlled update workflow for a specific container
func (o *Orchestrator) UpdateContainer(ctx context.Context, c *collector.ContainerInfo, opts UpdateOptions) (*UpdateReport, error) {
	start := time.Now()
	report := &UpdateReport{
		Target:      c.Name,
		Container:   c.Name,
		Project:     c.Project,
		Service:     c.Service,
		OldImageRef: c.ImageRef,
		OldImageID:  c.ImageID,
	}

	// 2. Guard Level Gate
	if (c.Guard == domain.GuardFrozen || c.Guard == domain.GuardLocked) && !opts.Force {
		report.Status = "refused"
		errMsg := fmt.Sprintf("容器 %s 受保护 (guard: %s)，如需更新请传递 --force 参数", c.Name, c.Guard)
		report.Message = errMsg
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", errMsg)
	}

	targetImage := ResolveTargetImage(c.ImageRef, opts.Tag, opts.Image)
	if opts.Tag == "" && opts.Image == "" && c.HasDrift && c.DeclaredImage != "" {
		targetImage = c.DeclaredImage
	}
	report.NewImageRef = targetImage

	// 3. Dry-run Check
	if opts.DryRun {
		report.Status = "dry_run"
		report.Message = fmt.Sprintf("预演：将容器 %s 镜像由 %s 更新至 %s", c.Name, c.ImageRef, targetImage)
		report.Duration = time.Since(start)
		return report, nil
	}

	// 4. Pull Image
	if err := o.pullImage(ctx, targetImage); err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("拉取新镜像 %s 失败: %v", targetImage, err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// Inspect pulled image to get new ImageID
	newImageID, err := o.inspectImage(ctx, targetImage)
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("获取新镜像 %s 信息失败: %v", targetImage, err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}
	report.NewImageID = newImageID

	// 5. Idempotent check
	if newImageID == c.ImageID && targetImage == c.ImageRef {
		report.Status = "skipped_identical"
		report.Message = fmt.Sprintf("容器 %s 镜像 ID 未变，已是最新状态，无需重启", c.Name)
		report.Duration = time.Since(start)
		return report, nil
	}

	// 6. GuardDB Snapshot Gate
	if c.Guard == domain.GuardDB || strings.ToLower(c.Kind) == "postgres" || strings.ToLower(c.Kind) == "redis" || strings.ToLower(c.Kind) == "gitea" || strings.ToLower(c.Kind) == "sqlite" {
		dumpResult, err := o.executeDump(ctx, c)
		if err != nil {
			report.Status = "dump_failed"
			report.Message = fmt.Sprintf("更新前强制数据快照失败，流程中止: %v", err)
			report.Duration = time.Since(start)
			return report, fmt.Errorf("%s", report.Message)
		}
		if dumpResult != nil && len(dumpResult.Artifacts) > 0 {
			report.DumpPath = dumpResult.Artifacts[0]
		}
	}

	// 7. Locate Compose File and Service
	composeFile, serviceName, err := o.locateComposeFileAndService(c)
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("定位 Compose 配置失败: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 8. Atomically update Compose YAML
	var modifyRes *compose.ModifyResult
	if targetImage != c.ImageRef {
		modifyRes, err = o.yamlModifier.UpdateImageTag(composeFile, serviceName, targetImage)
		if err != nil {
			report.Status = "failed"
			report.Message = fmt.Sprintf("改写 Compose YAML 失败: %v", err)
			report.Duration = time.Since(start)
			return report, fmt.Errorf("%s", report.Message)
		}
		report.BackupPath = modifyRes.BackupPath
	}

	// 9. Compose Up with --no-deps
	if err := o.runComposeUp(ctx, composeFile, serviceName, c.Project); err != nil {
		// Compose Up failed, rollback immediately
		_ = o.handleRollback(ctx, c, composeFile, serviceName, targetImage, modifyRes, fmt.Sprintf("compose up 失败: %v", err))
		report.Status = "rolled_back"
		report.Message = fmt.Sprintf("Compose Up 执行失败，已自动回滚: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 10. Wait for healthy
	healthy, probeMsg := o.waitForHealthy(ctx, c)
	if !healthy {
		// Probe failed, trigger automatic rollback
		rbErr := o.handleRollback(ctx, c, composeFile, serviceName, targetImage, modifyRes, probeMsg)
		report.Status = "rolled_back"
		report.Message = fmt.Sprintf("健康探针未通过 (%s)，已自动回滚 (回滚状态: %v)", probeMsg, rbErr)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 11. Record Success to State
	nowStr := time.Now().Format(time.RFC3339)
	backupRecord := report.BackupPath
	if backupRecord == "" {
		backupRecord = composeFile
	}
	entry := domain.HistoryEntry{
		At:         nowStr,
		Result:     "success",
		ImageID:    newImageID,
		ImageRef:   targetImage,
		PrevRef:    c.ImageRef,
		RefAfter:   targetImage,
		Note:       fmt.Sprintf("updated from %s to %s", c.ImageRef, targetImage),
		BackupPath: backupRecord,
	}
	_ = o.stateManager.RecordUpdate(c.Name, entry)

	report.Status = "success"
	report.Message = fmt.Sprintf("容器 %s 已成功更新至 %s 并且通过健康探针校验", c.Name, targetImage)
	report.Duration = time.Since(start)
	return report, nil
}

func (o *Orchestrator) locateComposeFileAndService(c *collector.ContainerInfo) (string, string, error) {
	serviceName := c.Service
	if serviceName == "" {
		serviceName = c.Name
	}

	composeFile := ""
	if len(c.Files) > 0 {
		composeFile = c.Files[0]
	}

	// Fallback to checking state.json history for BackupPath
	if composeFile == "" && o.stateManager != nil {
		if state, err := o.stateManager.Load(); err == nil {
			if cs, ok := state.Containers[c.Name]; ok {
				for i := len(cs.History) - 1; i >= 0; i-- {
					bp := cs.History[i].BackupPath
					if bp != "" {
						origFile := bp
						if strings.Contains(bp, ".ops-bak-") {
							origFile = strings.Split(bp, ".ops-bak-")[0]
						}
						composeFile = origFile
						break
					}
				}
			}
		}
	}

	if composeFile == "" {
		return "", "", fmt.Errorf("容器 %s 未关联 Compose 配置文件", c.Name)
	}

	// If serviceName equals container name, try to map to actual compose service name
	if serviceName == c.Name {
		proj, err := loader.LoadWithContext(context.Background(), types.ConfigDetails{
			WorkingDir:  filepath.Dir(composeFile),
			ConfigFiles: []types.ConfigFile{{Filename: composeFile}},
		}, loader.WithSkipValidation, func(opts *loader.Options) { opts.SetProjectName("res", true) })
		if err == nil {
			for name, svc := range proj.Services {
				if svc.ContainerName == c.Name {
					serviceName = name
					break
				}
			}
			if serviceName == c.Name && len(proj.Services) == 1 {
				for name := range proj.Services {
					serviceName = name
					break
				}
			}
		}
	}

	return composeFile, serviceName, nil
}

func (o *Orchestrator) pullImage(ctx context.Context, ref string) error {
	if o.imagePuller != nil {
		return o.imagePuller(ctx, ref)
	}
	if o.dockerClient == nil {
		return fmt.Errorf("docker client is nil")
	}
	resp, err := o.dockerClient.Engine().ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()
	_, _ = io.Copy(io.Discard, resp)
	return resp.Wait(ctx)
}

func (o *Orchestrator) inspectImage(ctx context.Context, ref string) (string, error) {
	if o.imageInspector != nil {
		return o.imageInspector(ctx, ref)
	}
	if o.dockerClient == nil {
		return "", fmt.Errorf("docker client is nil")
	}
	res, err := o.dockerClient.Engine().ImageInspect(ctx, ref)
	if err != nil {
		return "", err
	}
	return res.ID, nil
}

func (o *Orchestrator) tagImage(ctx context.Context, src, target string) error {
	if o.imageTagger != nil {
		return o.imageTagger(ctx, src, target)
	}
	if o.dockerClient == nil {
		return fmt.Errorf("docker client is nil")
	}
	_, err := o.dockerClient.Engine().ImageTag(ctx, client.ImageTagOptions{
		Source: src,
		Target: target,
	})
	return err
}

func (o *Orchestrator) executeDump(ctx context.Context, c *collector.ContainerInfo) (*dumper.DumpResult, error) {
	if o.dumper != nil {
		return o.dumper.Dump(ctx, c, o.backupBase)
	}
	dp, err := dumper.GetDumper(c.Kind, o.dockerClient)
	if err != nil {
		return nil, err
	}
	return dp.Dump(ctx, c, o.backupBase)
}

func (o *Orchestrator) runComposeUp(ctx context.Context, composeFile string, service string, projectName string) (upErr error) {
	if o.composeUpper != nil {
		return o.composeUpper(ctx, composeFile, service)
	}
	if o.dockerClient == nil {
		return fmt.Errorf("docker client is nil")
	}

	if projectName == "" {
		projectName = filepath.Base(filepath.Dir(composeFile))
	}

	proj, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir: filepath.Dir(composeFile),
		ConfigFiles: []types.ConfigFile{
			{Filename: composeFile},
		},
	}, func(opts *loader.Options) {
		opts.SetProjectName(projectName, true)
	})
	if err != nil {
		return fmt.Errorf("failed to load compose file %s: %w", composeFile, err)
	}

	// Resolve target service name: if not directly matching a compose service, resolve via ContainerName
	targetService := service
	if _, err := proj.GetService(targetService); err != nil {
		for name, svc := range proj.Services {
			if svc.ContainerName == service {
				targetService = name
				break
			}
		}
		if _, err := proj.GetService(targetService); err != nil && len(proj.Services) == 1 {
			for name := range proj.Services {
				targetService = name
				break
			}
		}
	}

	proj, err = proj.WithSelectedServices([]string{targetService}, types.IgnoreDependencies)
	if err != nil {
		return fmt.Errorf("failed to select service %s: %w", targetService, err)
	}

	absComposeFile, _ := filepath.Abs(composeFile)
	workingDir := filepath.Dir(absComposeFile)
	svc := proj.Services[targetService]
	if svc.CustomLabels == nil {
		svc.CustomLabels = make(map[string]string)
	}
	svc.CustomLabels[api.ProjectLabel] = projectName
	svc.CustomLabels[api.ServiceLabel] = targetService
	svc.CustomLabels[api.ConfigFilesLabel] = absComposeFile
	svc.CustomLabels[api.WorkingDirLabel] = workingDir
	svc.CustomLabels[api.OneoffLabel] = "False"
	proj.Services[targetService] = svc

	// Handle fixed container_name conflict and port occupation:
	// If the service specifies a fixed container_name, Docker daemon rejects creating
	// another container with the same name or binding the same port if the old one still exists.
	// We temporarily rename & stop the old container, and remove it after successful Up (or restore if Up fails).
	var oldContainerID string
	var origContainerName string
	var wasRunning bool
	svcConfig, _ := proj.GetService(targetService)
	if svcConfig.ContainerName != "" && o.dockerClient.Engine() != nil {
		origContainerName = svcConfig.ContainerName
		inspect, err := o.dockerClient.Engine().ContainerInspect(ctx, origContainerName, client.ContainerInspectOptions{})
		if err == nil && inspect.Container.ID != "" {
			oldContainerID = inspect.Container.ID
			wasRunning = inspect.Container.State != nil && inspect.Container.State.Running
			tempName := fmt.Sprintf("%s_ops_old_%d", origContainerName, time.Now().UnixNano()%1000000)
			if _, renameErr := o.dockerClient.Engine().ContainerRename(ctx, oldContainerID, client.ContainerRenameOptions{NewName: tempName}); renameErr == nil {
				if wasRunning {
					_, _ = o.dockerClient.Engine().ContainerStop(ctx, oldContainerID, client.ContainerStopOptions{})
				}
				defer func() {
					if upErr != nil {
						_, _ = o.dockerClient.Engine().ContainerRename(ctx, oldContainerID, client.ContainerRenameOptions{NewName: origContainerName})
						if wasRunning {
							_, _ = o.dockerClient.Engine().ContainerStart(ctx, oldContainerID, client.ContainerStartOptions{})
						}
					} else {
						_, _ = o.dockerClient.Engine().ContainerRemove(ctx, oldContainerID, client.ContainerRemoveOptions{Force: true})
					}
				}()
			}
		}
	}

	upErr = o.dockerClient.Compose().Up(ctx, proj, api.UpOptions{
		Create: api.CreateOptions{
			Services:             []string{targetService},
			Recreate:             api.RecreateForce,
			RecreateDependencies: api.RecreateNever,
			Inherit:              true,
		},
		Start: api.StartOptions{
			Services: []string{targetService},
		},
	})
	return upErr
}

func (o *Orchestrator) waitForHealthy(ctx context.Context, c *collector.ContainerInfo) (bool, string) {
	if o.waiter != nil {
		return o.waiter(ctx, c)
	}
	return probe.WaitForHealthy(ctx, o.dockerClient, c)
}

func (o *Orchestrator) handleRollback(ctx context.Context, c *collector.ContainerInfo, composeFile, service, failedImage string, modifyRes *compose.ModifyResult, reason string) error {
	timestamp := time.Now().Format("20060102150405")
	badTag := fmt.Sprintf("%s:ops-bad-%s", strings.Split(failedImage, ":")[0], timestamp)
	_ = o.tagImage(ctx, failedImage, badTag)

	// Restore Compose YAML
	if modifyRes != nil && modifyRes.BackupPath != "" {
		_ = o.yamlModifier.RestoreBackup(modifyRes.BackupPath, composeFile)
	}

	// Recreate container with old configuration
	_ = o.runComposeUp(ctx, composeFile, service, c.Project)

	// Wait for rollback container to come back up healthy
	_, _ = o.waitForHealthy(ctx, c)

	// Record to state.json
	entry := domain.HistoryEntry{
		At:       time.Now().Format(time.RFC3339),
		Result:   "rolled-back",
		ImageRef: failedImage,
		PrevRef:  c.ImageRef,
		RefAfter: c.ImageRef,
		Note:     fmt.Sprintf("rolled back due to: %s (bad tag: %s)", reason, badTag),
	}
	if modifyRes != nil {
		entry.BackupPath = modifyRes.BackupPath
	}
	_ = o.stateManager.RecordUpdate(c.Name, entry)

	return nil
}
