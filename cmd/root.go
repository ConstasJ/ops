package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"ops-go/pkg/docker"
	"ops-go/pkg/domain"
)

var (
	socketFlag string
	configFlag string
	stateFlag  string
	verboseFlag bool

	cachedDockerClient *docker.Client
)

// NewRootCommand creates the top-level ops-go Cobra command
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ops",
		Short: "ops — 受控容器运维工具箱 (Go 移植版)",
		Long: `ops 是面向 Docker 与 Docker Compose 服务的受控运维工具箱。
提供带前置快照门禁的安全更新、零宿主依赖数据库 dump、健康探针校验、以及原子故障回滚。`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&socketFlag, "socket", "H", "", "Docker daemon socket URL 或路径 (默认遵循 DOCKER_HOST 或标准 socket)")
	rootCmd.PersistentFlags().StringVarP(&configFlag, "config", "c", "/opt/ops/containers.json", "containers.json 容器知识库配置文件路径")
	rootCmd.PersistentFlags().StringVarP(&stateFlag, "state", "s", "/var/lib/ops/state.json", "state.json 状态与回滚记录文件路径")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "启用详细调试日志")

	// Add subcommands
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newPingCommand())

	return rootCmd
}

// Execute runs the root command
func Execute() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// GetDockerClient returns a shared or newly initialized DockerClient
func GetDockerClient(ctx context.Context) (*docker.Client, error) {
	if cachedDockerClient != nil {
		return cachedDockerClient, nil
	}
	cli, err := docker.NewClient(ctx, socketFlag)
	if err != nil {
		return nil, err
	}
	cachedDockerClient = cli
	return cli, nil
}

// GetConfig loads the containers.json configuration
func GetConfig() (*domain.ContainersConfig, error) {
	return domain.LoadConfig(configFlag)
}

// GetStateManager returns a StateManager targeting the configured state path
func GetStateManager() *domain.StateManager {
	return domain.NewStateManager(stateFlag)
}
