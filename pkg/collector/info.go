package collector

import (
	"fmt"
	"time"

	"ops-go/pkg/domain"
)

// ContainerInfo represents the unified operational metadata of a container
type ContainerInfo struct {
	ID           string
	Name         string
	ImageRef     string
	ImageID      string
	State        string // "running", "exited", "restarting", etc.
	Health       string // "healthy", "unhealthy", "starting", ""
	RestartCount int
	ExitCode     int
	StartedAt    time.Time
	Project      string
	Service      string
	Files        []string
	WorkingDir   string
	Auto         bool

	DeclaredImage string // Compose 文件中声明的镜像引用
	HasDrift      bool   // 声明镜像与当前运行中容器镜像是否存在配置漂移

	Kind      string
	Guard     domain.GuardLevel
	Notes     string
	Probe     *domain.ProbeSpec
	Timeout   time.Duration
	Settle    time.Duration
	ExtraDump string
}

// StackService returns "project/service" if managed by Compose, otherwise "name"
func (c *ContainerInfo) StackService() string {
	if c.Project != "" && c.Service != "" {
		return fmt.Sprintf("%s/%s", c.Project, c.Service)
	}
	return c.Name
}

// IsHealthy returns true if the container is running and healthy (or has no healthcheck)
func (c *ContainerInfo) IsHealthy() bool {
	if c.State != "running" {
		return false
	}
	return c.Health == "healthy" || c.Health == ""
}

// FormatUptime returns human-readable duration since started
func (c *ContainerInfo) FormatUptime() string {
	if c.StartedAt.IsZero() || c.State != "running" {
		return "-"
	}
	d := time.Since(c.StartedAt)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
