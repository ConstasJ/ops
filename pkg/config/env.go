package config

import (
	"os"

	"github.com/joho/godotenv"
)

// LoadDotEnv 加载指定路径的 .env 文件；若 paths 为空，自动尝试加载当前目录下的 .env
// 注意：godotenv 遵循不覆盖现有环境变量的标准行为，确保 OS 环境变量的优先级高于 .env
func LoadDotEnv(paths ...string) error {
	if len(paths) == 0 {
		// 自动检查默认位置
		candidates := []string{".env", "/etc/ops/.env", "/opt/ops/.env"}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return godotenv.Load(p)
			}
		}
		return nil
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// ResolvePrecedence 根据严格的优先级返回最终有效的配置字符串：
// 命令行参数 (flag) > 系统环境变量 (env) > 配置文件/dotenv (cfg) > 默认内置值 (def)
func ResolvePrecedence(flagVal, envVal, cfgVal, defVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	if cfgVal != "" {
		return cfgVal
	}
	return defVal
}
