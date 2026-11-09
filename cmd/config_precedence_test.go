package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestConfigPrecedence_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. 创建测试配置文件 ops.yaml
	yamlPath := filepath.Join(tmpDir, "ops.yaml")
	yamlContent := `
socket: "tcp://from-config:2375"
state: "/tmp/from-config/state.json"
`
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// 测试 A: 仅从配置文件加载
	{
		root := cmd.NewRootCommand()
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"--config", yamlPath, "ping"})
		_ = root.Execute()

		appCfg := cmd.GetAppConfig()
		assert.Equal(t, "tcp://from-config:2375", appCfg.Socket)
	}

	// 测试 B: 环境变量覆盖配置文件
	{
		_ = os.Setenv("OPS_SOCKET", "tcp://from-os-env:2375")
		defer os.Unsetenv("OPS_SOCKET")

		root := cmd.NewRootCommand()
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"--config", yamlPath, "ping"})
		_ = root.Execute()

		// 检查底层连接的 socket 是否优先采纳了环境变量
		// (通过 ping 错误信息中是否包含 from-os-env 验证)
		assert.Contains(t, buf.String(), "from-os-env")
	}

	// 测试 C: CLI 参数 (-H) 覆盖环境变量
	{
		_ = os.Setenv("OPS_SOCKET", "tcp://from-os-env:2375")
		defer os.Unsetenv("OPS_SOCKET")

		root := cmd.NewRootCommand()
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"-H", "tcp://from-flag:2375", "--config", yamlPath, "ping"})
		_ = root.Execute()

		assert.Contains(t, buf.String(), "from-flag")
	}
}
