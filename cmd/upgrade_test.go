package cmd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestUpgradeCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()

	upgradeCmd, _, err := root.Find([]string{"upgrade"})
	require.NoError(t, err)
	assert.Equal(t, "upgrade", upgradeCmd.Name())
	assert.NotNil(t, upgradeCmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, upgradeCmd.Flags().Lookup("yes"))
	assert.NotNil(t, upgradeCmd.Flags().Lookup("only-auto"))
	assert.NotNil(t, upgradeCmd.Flags().Lookup("notify"))
	assert.NotNil(t, upgradeCmd.Flags().Lookup("report-dir"))
}

func TestUpgradeCommand_Offline(t *testing.T) {
	buf := new(bytes.Buffer)
	root := cmd.NewRootCommand()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"upgrade", "--socket", "http://127.0.0.1:59999", "--dry-run"})

	err := root.Execute()
	assert.Error(t, err)
}
