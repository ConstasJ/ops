package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"ops-go/pkg/config"
	"ops-go/pkg/docker"
	"ops-go/pkg/domain"
)

var (
	socketFlag  string
	configFlag  string
	stateFlag   string
	verboseFlag bool

	loadedAppConfig    *config.AppConfig
	resolvedSocket     string
	resolvedConfigPath string
	resolvedState      string

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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cachedDockerClient = nil
			loadedAppConfig = nil
			// 1. 自动尝试加载 .env 文件（不覆盖已有环境变量）
			_ = config.LoadDotEnv()

			// 2. 依照搜索链定位配置文件 (命令行参数 > OPS_CONFIG > 自动探测)
			explicitConfig := ""
			if cmd.Flags().Changed("config") {
				explicitConfig = configFlag
			}
			resolvedConfigPath = config.FindConfigFile(explicitConfig)

			// 3. 若找到配置文件，执行加载
			if resolvedConfigPath != "" {
				if appCfg, err := config.LoadFile(resolvedConfigPath); err == nil {
					loadedAppConfig = appCfg
				} else if explicitConfig != "" {
					// 仅在显式指定了 -c 却加载失败时抛错
					return fmt.Errorf("加载指定配置文件失败: %w", err)
				}
			}

			// 4. 解析 Socket 优先级: Flag > OPS_SOCKET / DOCKER_HOST > ConfigFile > Default("")
			var flagSocket string
			if cmd.Flags().Changed("socket") {
				flagSocket = socketFlag
			}
			envSocket := os.Getenv("OPS_SOCKET")
			if envSocket == "" {
				envSocket = os.Getenv("DOCKER_HOST")
			}
			cfgSocket := ""
			if loadedAppConfig != nil {
				cfgSocket = loadedAppConfig.Socket
			}
			resolvedSocket = config.ResolvePrecedence(flagSocket, envSocket, cfgSocket, "")

			// 5. 解析 State 文件路径优先级: Flag > OPS_STATE > ConfigFile > Default("/var/lib/ops/state.json")
			var flagState string
			if cmd.Flags().Changed("state") {
				flagState = stateFlag
			}
			envState := os.Getenv("OPS_STATE")
			cfgState := ""
			if loadedAppConfig != nil {
				cfgState = loadedAppConfig.State
			}
			resolvedState = config.ResolvePrecedence(flagState, envState, cfgState, "/var/lib/ops/state.json")

			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&socketFlag, "socket", "H", "", "Docker daemon socket URL 或路径 (默认遵循 DOCKER_HOST 或标准 socket)")
	rootCmd.PersistentFlags().StringVarP(&configFlag, "config", "c", "", "配置文件路径 (支持 json/yaml/toml，未指定时按标准链自动发现)")
	rootCmd.PersistentFlags().StringVarP(&stateFlag, "state", "s", "", "state.json 状态与回滚记录文件路径 (未指定时遵循配置或默认 /var/lib/ops/state.json)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "启用详细调试日志")

	// Add subcommands
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newPingCommand())
	rootCmd.AddCommand(newStatusCommand())
	rootCmd.AddCommand(newLsCommand())
	rootCmd.AddCommand(newNotesCommand())
	rootCmd.AddCommand(newDoctorCommand())
	rootCmd.AddCommand(newDumpCommand())
	rootCmd.AddCommand(newPendingCommand())
	rootCmd.AddCommand(newUpdateCommand())
	rootCmd.AddCommand(newUpgradeCommand())
	rootCmd.AddCommand(newRollbackCommand())

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
	cli, err := docker.NewClient(ctx, resolvedSocket)
	if err != nil {
		return nil, err
	}
	cachedDockerClient = cli
	return cli, nil
}

// GetAppConfig returns the loaded AppConfig, or an empty instance if none was loaded
func GetAppConfig() *config.AppConfig {
	if loadedAppConfig != nil {
		return loadedAppConfig
	}
	return config.NewAppConfig()
}

// GetConfig loads the containers configuration from AppConfig or file
func GetConfig() (*domain.ContainersConfig, error) {
	if loadedAppConfig != nil && len(loadedAppConfig.Containers) > 0 {
		return &domain.ContainersConfig{Specs: loadedAppConfig.Containers}, nil
	}

	// 如果未能预加载但指定了解析路径，尝试独立加载
	if resolvedConfigPath != "" {
		if appCfg, err := config.LoadFile(resolvedConfigPath); err == nil && len(appCfg.Containers) > 0 {
			return &domain.ContainersConfig{Specs: appCfg.Containers}, nil
		}
	}

	// 平稳回退到空配置
	return &domain.ContainersConfig{Specs: make(map[string]domain.ContainerSpec)}, nil
}

// GetStateManager returns a StateManager targeting the configured state path
func GetStateManager() *domain.StateManager {
	targetState := resolvedState
	if targetState == "" {
		targetState = "/var/lib/ops/state.json"
	}
	return domain.NewStateManager(targetState)
}
