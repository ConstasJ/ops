package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
	"ops-go/pkg/domain"
)

// LoadFile 加载并解析指定路径的配置文件，支持 JSON、YAML、TOML 格式，
// 自动兼容纯容器映射（如旧版 containers.json）与全功能 AppConfig（如 ops.yaml）
func LoadFile(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败 (%s): %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	cfg := NewAppConfig()

	// 1. 先尝试按全量 AppConfig（含有 containers: 等顶层键）解码
	var unmarshalErr error
	switch ext {
	case ".yaml", ".yml":
		unmarshalErr = yaml.Unmarshal(data, cfg)
	case ".toml":
		unmarshalErr = toml.Unmarshal(data, cfg)
	case ".json":
		unmarshalErr = json.Unmarshal(data, cfg)
	default:
		if err := yaml.Unmarshal(data, cfg); err == nil && len(cfg.Containers) > 0 {
			unmarshalErr = nil
		} else if err := json.Unmarshal(data, cfg); err == nil && len(cfg.Containers) > 0 {
			unmarshalErr = nil
		} else {
			unmarshalErr = toml.Unmarshal(data, cfg)
		}
	}

	// 若顶层成功解析出 containers 或全局关键字段，清理注释后返回
	if unmarshalErr == nil && (len(cfg.Containers) > 0 || cfg.Socket != "" || cfg.State != "" || cfg.ReportDir != "") {
		for k := range cfg.Containers {
			if strings.HasPrefix(k, "_") {
				delete(cfg.Containers, k)
			}
		}
		return cfg, nil
	}

	// 2. 若不满足全量 AppConfig，尝试按纯容器字典解码（兼容带有 _comment 等任意注释键的旧版 containers.json）
	if parseContainersMap(data, ext, cfg) {
		return cfg, nil
	}

	if unmarshalErr != nil {
		return nil, fmt.Errorf("解析配置文件格式失败 (%s): %w", path, unmarshalErr)
	}
	return cfg, nil
}

func parseContainersMap(data []byte, ext string, cfg *AppConfig) bool {
	switch ext {
	case ".yaml", ".yml":
		var rawMap map[string]any
		if err := yaml.Unmarshal(data, &rawMap); err == nil {
			for k := range rawMap {
				if strings.HasPrefix(k, "_") {
					continue
				}
				// 重新编码为 单个 ContainerSpec
				subData, _ := yaml.Marshal(rawMap[k])
				var spec domain.ContainerSpec
				if err := yaml.Unmarshal(subData, &spec); err == nil {
					cfg.Containers[k] = spec
				}
			}
			return len(cfg.Containers) > 0
		}
	case ".toml":
		var rawMap map[string]any
		if err := toml.Unmarshal(data, &rawMap); err == nil {
			for k := range rawMap {
				if strings.HasPrefix(k, "_") {
					continue
				}
				subData, _ := toml.Marshal(rawMap[k])
				var spec domain.ContainerSpec
				if err := toml.Unmarshal(subData, &spec); err == nil {
					cfg.Containers[k] = spec
				}
			}
			return len(cfg.Containers) > 0
		}
	default:
		// 默认按 JSON 处理
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(data, &rawMap); err == nil {
			for k, v := range rawMap {
				if strings.HasPrefix(k, "_") {
					continue
				}
				var spec domain.ContainerSpec
				if err := json.Unmarshal(v, &spec); err == nil {
					cfg.Containers[k] = spec
				}
			}
			return len(cfg.Containers) > 0
		}
	}
	return false
}

// FindConfigFile 依照标准搜索链查找可用配置文件路径：
// 命令行显式参数 > 环境变量 OPS_CONFIG > 标准路径自动发现 (./, /etc/ops/, /opt/ops/)
func FindConfigFile(explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}

	if envPath := os.Getenv("OPS_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	candidates := []string{
		// 当前目录优先
		"./ops.yaml", "./ops.yml", "./ops.toml", "./ops.json",
		"./containers.yaml", "./containers.yml", "./containers.toml", "./containers.json",
		// FHS 3.0 标准目录
		"/etc/ops/ops.yaml", "/etc/ops/ops.yml", "/etc/ops/ops.toml", "/etc/ops/ops.json",
		"/etc/ops/containers.yaml", "/etc/ops/containers.toml", "/etc/ops/containers.json",
		// 历史兼容路径
		"/opt/ops/containers.json",
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}
