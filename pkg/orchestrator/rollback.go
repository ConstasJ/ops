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
		if cs, ok := state.Containers[c.StackService()]; ok && len(cs.History) > 0 {
			containerState = cs
			exists = true
		} else if cs2, ok2 := state.Containers[c.Service]; ok2 && len(cs2.History) > 0 {
			containerState = cs2
			exists = true
		}
	}
	if !exists || len(containerState.History) == 0 {
		report.Status = "failed"
		report.Message = fmt.Sprintf("state.json 里没有容器 %s 的记录，无法确定回滚目标", c.Name)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// Filter successful entries
	var successHist []domain.HistoryEntry
	for _, h := range containerState.History {
		if h.Result == "success" {
			successHist = append(successHist, h)
		}
	}

	var targetRef string
	var targetID string

	if len(successHist) > 0 {
		latest := successHist[len(successHist)-1]
		// Drift case: running container ImageID is non-empty and differs from latest recorded success ImageID
		if c.ImageID != "" && latest.ImageID != "" && c.ImageID != latest.ImageID {
			targetID = latest.ImageID
			targetRef = latest.ImageRef
			if targetRef == "" {
				targetRef = c.ImageRef
			}
		} else {
			// Normal undo rollback
			if len(successHist) >= 2 {
				prev := successHist[len(successHist)-2]
				targetRef = prev.ImageRef
				targetID = prev.ImageID
			} else if latest.PrevRef != "" {
				targetRef = latest.PrevRef
			}
		}
	}

	// Fallback to checking any history entry with PrevRef
	if targetRef == "" && targetID == "" {
		for i := len(containerState.History) - 1; i >= 0; i-- {
			entry := containerState.History[i]
			if entry.PrevRef != "" && entry.PrevRef != c.ImageRef {
				targetRef = entry.PrevRef
				targetID = entry.ImageID
				break
			}
		}
	}

	if targetRef == "" && targetID == "" {
		report.Status = "failed"
		report.Message = fmt.Sprintf("容器 %s 无可用回滚目标版本", c.Name)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	if targetRef == "" {
		targetRef = c.ImageRef
	}

	report.RestoredRef = targetRef
	report.RestoredID = targetID

	// 3. Locate Compose File and Service
	composeFile, serviceName, err := o.locateComposeFileAndService(c)
	if err != nil {
		report.Status = "failed"
		report.Message = fmt.Sprintf("定位 Compose 配置失败: %v", err)
		report.Duration = time.Since(start)
		return report, fmt.Errorf("%s", report.Message)
	}

	// 4. Update Compose YAML tag if targetRef differs
	if targetRef != c.ImageRef {
		_, err := o.yamlModifier.UpdateImageTag(composeFile, serviceName, targetRef)
		if err != nil {
			report.Status = "failed"
			report.Message = fmt.Sprintf("改写 Compose YAML 失败: %v", err)
			report.Duration = time.Since(start)
			return report, fmt.Errorf("%s", report.Message)
		}
	}

	// 5. If targetID is available, retag targetID to targetRef (ensuring local target is right)
	if targetID != "" {
		_ = o.tagImage(ctx, targetID, targetRef)
	} else if targetRef != "" {
		// Pull if necessary
		_ = o.pullImage(ctx, targetRef)
	}

	// 6. Run Compose Up --no-deps
	if err := o.runComposeUp(ctx, composeFile, serviceName, c.Project); err != nil {
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
		At:       time.Now().Format(time.RFC3339),
		Result:   "rolled-back",
		ImageID:  targetID,
		ImageRef: targetRef,
		PrevRef:  c.ImageRef,
		RefAfter: targetRef,
		Note:     fmt.Sprintf("rolled back to %s (%s)", targetRef, targetID),
	}
	_ = o.stateManager.RecordUpdate(c.Name, entry)

	report.Status = "success"
	report.Message = fmt.Sprintf("容器 %s 已成功回退至 %s", c.Name, targetRef)
	report.Duration = time.Since(start)
	return report, nil
}
