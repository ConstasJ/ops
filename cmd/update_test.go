package cmd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestUpdateCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()
	updateCmd, _, err := root.Find([]string{"update"})
	require.NoError(t, err)
	assert.Equal(t, "update", updateCmd.Name())

	assert.NotNil(t, updateCmd.Flags().Lookup("tag"))
	assert.NotNil(t, updateCmd.Flags().Lookup("image"))
	assert.NotNil(t, updateCmd.Flags().Lookup("force"))
	assert.NotNil(t, updateCmd.Flags().Lookup("dry-run"))
}

func TestRollbackCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()
	rollbackCmd, _, err := root.Find([]string{"rollback"})
	require.NoError(t, err)
	assert.Equal(t, "rollback", rollbackCmd.Name())
}

func TestUpdateCommand_FlagParsing(t *testing.T) {
	t.Run("Parse tag flag", func(t *testing.T) {
		root := cmd.NewRootCommand()
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs([]string{"update", "linknet/db", "--tag", "0.8.7", "--dry-run"})

		// We execute dry-run with no docker daemon running; it may fail at docker connection
		// but flag parsing must have succeeded
		updateCmd, _, err := root.Find([]string{"update"})
		require.NoError(t, err)
		require.NoError(t, updateCmd.ParseFlags([]string{"--tag", "0.8.7", "--dry-run"}))

		tagVal, err := updateCmd.Flags().GetString("tag")
		require.NoError(t, err)
		assert.Equal(t, "0.8.7", tagVal)

		dryRunVal, err := updateCmd.Flags().GetBool("dry-run")
		require.NoError(t, err)
		assert.True(t, dryRunVal)
	})

	t.Run("Parse force flag", func(t *testing.T) {
		root := cmd.NewRootCommand()
		updateCmd, _, err := root.Find([]string{"update"})
		require.NoError(t, err)
		require.NoError(t, updateCmd.ParseFlags([]string{"--force"}))

		forceVal, err := updateCmd.Flags().GetBool("force")
		require.NoError(t, err)
		assert.True(t, forceVal)
	})
}
