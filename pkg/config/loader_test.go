package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/domain"
)

func TestLoadFile_LegacyContainersJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "containers.json")

	content := `{
		"_comment": "测试配置文件",
		"db-service": {
			"kind": "postgres",
			"guard": "db",
			"timeout": 150,
			"probe": {
				"tcp": ":5432"
			}
		}
	}`
	err := os.WriteFile(jsonPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := LoadFile(jsonPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Contains(t, cfg.Containers, "db-service")
	assert.Equal(t, domain.GuardDB, cfg.Containers["db-service"].Guard)
	assert.Equal(t, "postgres", cfg.Containers["db-service"].Kind)
	assert.Equal(t, 150, cfg.Containers["db-service"].Timeout)
}

func TestLoadFile_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "ops.yaml")

	content := `
socket: "unix:///var/run/docker.sock"
state: "/var/lib/ops/state.json"
report_dir: "/var/log/ops"
backup_dir: "/var/backups/ops"

notify:
  feishu_webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/test"
  smtp:
    host: "smtp.example.com"
    port: "587"
    user: "mailer@example.com"
    to: ["admin@example.com"]

containers:
  web-service:
    guard: assisted
    probe:
      http: "http://:8080/health"
    timeout: 60
`
	err := os.WriteFile(yamlPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := LoadFile(yamlPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "unix:///var/run/docker.sock", cfg.Socket)
	assert.Equal(t, "/var/lib/ops/state.json", cfg.State)
	assert.Equal(t, "/var/log/ops", cfg.ReportDir)
	assert.Equal(t, "https://open.feishu.cn/open-apis/bot/v2/hook/test", cfg.Notify.FeishuWebhook)
	assert.Equal(t, "smtp.example.com", cfg.Notify.SMTP.Host)

	require.Contains(t, cfg.Containers, "web-service")
	assert.Equal(t, domain.GuardAssisted, cfg.Containers["web-service"].Guard)
	assert.Equal(t, "http://:8080/health", cfg.Containers["web-service"].Probe.HTTP)
}

func TestLoadFile_TOML(t *testing.T) {
	tmpDir := t.TempDir()
	tomlPath := filepath.Join(tmpDir, "ops.toml")

	content := `
socket = "tcp://127.0.0.1:2375"
report_dir = "/var/log/ops"

[notify]
dingtalk_webhook = "https://oapi.dingtalk.com/robot/send?access_token=test"

[containers.redis-service]
guard = "db"
kind = "redis"
timeout = 30
`
	err := os.WriteFile(tomlPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := LoadFile(tomlPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "tcp://127.0.0.1:2375", cfg.Socket)
	assert.Equal(t, "https://oapi.dingtalk.com/robot/send?access_token=test", cfg.Notify.DingTalkWebhook)
	require.Contains(t, cfg.Containers, "redis-service")
	assert.Equal(t, domain.GuardDB, cfg.Containers["redis-service"].Guard)
	assert.Equal(t, "redis", cfg.Containers["redis-service"].Kind)
}

func TestLoadDotEnv_NoOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	err := os.WriteFile(envPath, []byte("EXISTING_KEY=from_env_file\nNEW_KEY=from_dotenv\n"), 0644)
	require.NoError(t, err)

	_ = os.Setenv("EXISTING_KEY", "already_in_os")
	defer os.Unsetenv("EXISTING_KEY")
	defer os.Unsetenv("NEW_KEY")

	err = LoadDotEnv(envPath)
	require.NoError(t, err)

	// EXISTING_KEY 必须保留来自 OS 的值（环境变量优先于 .env）
	assert.Equal(t, "already_in_os", os.Getenv("EXISTING_KEY"))
	// NEW_KEY 从 .env 文件中加载
	assert.Equal(t, "from_dotenv", os.Getenv("NEW_KEY"))
}

func TestResolveConfigWithPrecedence(t *testing.T) {
	// 验证优先级：Flag > Env > .env > ConfigFile > Default
	flagVal := "flag-socket"
	envVal := "env-socket"
	cfgVal := "cfg-socket"
	defaultVal := "default-socket"

	// 1. Flag 优先级最高
	val1 := ResolvePrecedence(flagVal, envVal, cfgVal, defaultVal)
	assert.Equal(t, "flag-socket", val1)

	// 2. Flag 为空时，Env 胜出
	val2 := ResolvePrecedence("", envVal, cfgVal, defaultVal)
	assert.Equal(t, "env-socket", val2)

	// 3. Flag 和 Env 均为空时，ConfigFile 胜出
	val3 := ResolvePrecedence("", "", cfgVal, defaultVal)
	assert.Equal(t, "cfg-socket", val3)

	// 4. 全为空时，Default 胜出
	val4 := ResolvePrecedence("", "", "", defaultVal)
	assert.Equal(t, "default-socket", val4)
}
