package orchestrator

import (
	"context"
	"fmt"
	"time"

	"ops-go/pkg/collector"
	"ops-go/pkg/domain"
)

// RollbackReport details the result of a rollback operation
type RollbackReport struct {
	Target      string        `json:"target"`
	Container   string        `json:"container"`
	Project     string        `json:"project"`
	Service     string        `json:"service"`
	RestoredRef string        `json:"restored_ref"`
	RestoredID  string        `json:"restored_id"`
	Status      string        `json:"status"` // "success", "failed"
	Message     string        `json:"message"`
	Duration    time.Duration `json:"duration"`
}

// Rollback restores a container to its previous recorded image version
func (o *Orchestrator) Rollback(ctx context.Context, target string) (*RollbackReport, error) {
	start := time.Now()
	report := &RollbackReport{
		Target: target,
	}

	// 1. Locate container
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
		report.Status = "failed"
		report.Message = err.Error()
		report.Duration = time.Since(start)
		return report, err
	}

	report.Container = c.Name
	report.Project = c.Project
	report.Service = c.Service

	// 2. Load state to find previous image version
	state, err := o.stateManager.Load()
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("读取状态文件失败: %v", err)
		report.Duration = time.Since(start)
		return report, err
	}

	containerState, exists := state.Containers[c.Name]
	if !exists || len(containerState.History) == 0 {
		report.Status = "failed"
		report.Message = fmt.Sprintf("容器 %s 无可用回滚历史记录", c.Name)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// Find the most recent entry with a valid PrevRef
	var targetPrevRef string
	for i := len(containerState.History) - 1; i >= 0; i-- {
		entry := containerState.History[i]
		if entry.PrevRef != "" && entry.PrevRef != c.ImageRef {
			targetPrevRef = entry.PrevRef
			break
		}
	}

	if targetPrevRef == "" {
		report.Status = "failed"
		report.Message = fmt.Sprintf("未在历史记录中找到与当前镜像不同的上一版本")
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	report.RestoredRef = targetPrevRef

	// 3. Ensure previous image is pulled
	if err := o.pullImage(ctx, targetPrevRef); err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("拉取回滚镜像 %s 失败: %v", targetPrevRef, err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	restoredID, err := o.inspectImage(ctx, targetPrevRef)
	if err == nil {
		report.RestoredID = restoredID
	}

	// 4. Locate Compose File and Service
	composeFile, serviceName, err := o.locateComposeFileAndService(c)
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("定位 Compose 配置失败: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 5. Update Compose YAML
	modifyRes, err := o.yamlModifier.UpdateImageTag(composeFile, serviceName, targetPrevRef)
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("改写 Compose YAML 失败: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 6. Run Compose Up --no-deps
	if err := o.runComposeUp(ctx, composeFile, serviceName); err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("Compose Up 启动回滚容器失败: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 7. Wait for Healthy
	healthy, probeMsg := o.waitForHealthy(ctx, c)
	if !healthy {
		report.Status = "failed"
		report.Message = fmt.Sprintf("回滚容器启动后健康探针未通过: %s", probeMsg)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 8. Record Rollback to state.json
	entry := domain.HistoryEntry{
		At:         time.Now().Format(time.RFC3339),
		Result:     "rolled-back",
		ImageID:    report.RestoredID,
		ImageRef:   targetPrevRef,
		PrevRef:    c.ImageRef,
		RefAfter:   targetPrevRef,
		Note:       fmt.Sprintf("manually rolled back to %s", targetPrevRef),
		BackupPath: modifyRes.BackupPath,
	}
	_ = o.stateManager.RecordUpdate(c.Name, entry)

	report.Status = "success"
	report.Message = fmt.Sprintf("容器 %s 已成功回滚至 %s", c.Name, targetPrevRef)
	report.Duration = time.Since(start)
	return report, nil
}
