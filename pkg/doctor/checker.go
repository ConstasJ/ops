package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
	"ops-go/pkg/domain"
)

// Level indicates severity of a health finding
type Level string

const (
	LevelOK    Level = "OK"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Issue records a specific diagnostic finding
type Issue struct {
	Level     Level
	Container string
	Title     string
	Detail    string
}

// ContainerReport contains findings from container inspection
type ContainerReport struct {
	Issues     []Issue
	WarnCount  int
	ErrorCount int
}

// BackupStatus records freshness for a single database container
type BackupStatus struct {
	Container string
	Kind      string
	Level     Level
	Detail    string
	LatestAt  time.Time
}

// BackupReport contains backup freshness results
type BackupReport struct {
	Statuses   []BackupStatus
	WarnCount  int
	ErrorCount int
}

// Checker runs system diagnostics
type Checker struct {
	backupRoot   string
	dockerClient *docker.Client
}

// NewChecker creates a new health checker
func NewChecker(backupRoot string, dockerClient *docker.Client) *Checker {
	if backupRoot == "" {
		backupRoot = "/var/backups/ops"
	}
	return &Checker{
		backupRoot:   backupRoot,
		dockerClient: dockerClient,
	}
}

// CheckContainers analyzes container statuses, healthcheck states, and restart flapping
func (chk *Checker) CheckContainers(containers []collector.ContainerInfo) ContainerReport {
	var rep ContainerReport

	for _, c := range containers {
		// 1. Exited/Stopped containers
		if c.State != "running" {
			rep.Issues = append(rep.Issues, Issue{
				Level:     LevelError,
				Container: c.Name,
				Title:     "容器未处于运行状态",
				Detail:    fmt.Sprintf("当前状态: %s (退出码: %d)", c.State, c.ExitCode),
			})
			rep.ErrorCount++
			continue
		}

		// 2. Healthcheck status
		if c.Health == "unhealthy" {
			rep.Issues = append(rep.Issues, Issue{
				Level:     LevelError,
				Container: c.Name,
				Title:     "健康门禁处于 unhealthy 状态",
				Detail:    "内置或探针健康检查未通过",
			})
			rep.ErrorCount++
		} else if c.Health == "starting" {
			rep.Issues = append(rep.Issues, Issue{
				Level:     LevelWarn,
				Container: c.Name,
				Title:     "健康门禁仍处于 starting 启动等待中",
				Detail:    "容器可能刚启动或探针反应迟缓",
			})
			rep.WarnCount++
		}

		// 3. Restart flapping
		if c.RestartCount >= 3 {
			rep.Issues = append(rep.Issues, Issue{
				Level:     LevelWarn,
				Container: c.Name,
				Title:     "发生过频繁重启",
				Detail:    fmt.Sprintf("累计重启次数: %d", c.RestartCount),
			})
			rep.WarnCount++
		}
	}

	return rep
}

// CheckBackups verifies that GuardDB containers have recent snapshots (< 48 hours)
func (chk *Checker) CheckBackups(containers []collector.ContainerInfo) BackupReport {
	var rep BackupReport

	for _, c := range containers {
		if c.Guard != domain.GuardDB {
			continue
		}

		dir := filepath.Join(chk.backupRoot, c.Name)
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			rep.Statuses = append(rep.Statuses, BackupStatus{
				Container: c.Name,
				Kind:      c.Kind,
				Level:     LevelError,
				Detail:    "从未检测到快照备份或目录不存在",
			})
			rep.ErrorCount++
			continue
		}

		// Collect snapshot directories
		var snapDirs []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				snapDirs = append(snapDirs, e.Name())
			}
		}

		if len(snapDirs) == 0 {
			rep.Statuses = append(rep.Statuses, BackupStatus{
				Container: c.Name,
				Kind:      c.Kind,
				Level:     LevelError,
				Detail:    "未检测到有效快照子目录",
			})
			rep.ErrorCount++
			continue
		}

		sort.Strings(snapDirs)
		latestName := snapDirs[len(snapDirs)-1]
		latestPath := filepath.Join(dir, latestName)

		// Try to read timestamp from meta.json, or directory name, or modTime
		var snapshotTime time.Time
		if metaData, err := os.ReadFile(filepath.Join(latestPath, "meta.json")); err == nil {
			var meta struct {
				At string `json:"at"`
			}
			if json.Unmarshal(metaData, &meta) == nil && meta.At != "" {
				if t, err := time.Parse("2006-01-02T15:04:05", meta.At); err == nil {
					snapshotTime = t
				}
			}
		}

		if snapshotTime.IsZero() {
			// Try parsing directory name format "%Y%m%d-%H%M%S"
			if t, err := time.Parse("20060102-150405", latestName); err == nil {
				snapshotTime = t
			}
		}

		if snapshotTime.IsZero() {
			if info, err := os.Stat(latestPath); err == nil {
				snapshotTime = info.ModTime()
			}
		}

		age := time.Since(snapshotTime)
		if age > 48*time.Hour {
			rep.Statuses = append(rep.Statuses, BackupStatus{
				Container: c.Name,
				Kind:      c.Kind,
				Level:     LevelWarn,
				Detail:    fmt.Sprintf("最新快照距今已超过 48 小时 (%s, 约 %d 小时前)", latestName, int(age.Hours())),
				LatestAt:  snapshotTime,
			})
			rep.WarnCount++
		} else {
			rep.Statuses = append(rep.Statuses, BackupStatus{
				Container: c.Name,
				Kind:      c.Kind,
				Level:     LevelOK,
				Detail:    fmt.Sprintf("最新快照健康 (%s, %s 前)", latestName, formatAge(age)),
				LatestAt:  snapshotTime,
			})
		}
	}

	return rep
}

func formatAge(d time.Duration) string {
	if d.Hours() >= 24 {
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
