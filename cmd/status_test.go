package cmd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestStatusAndLsCommands_Registered(t *testing.T) {
	root := cmd.NewRootCommand()

	statusCmd, _, err := root.Find([]string{"status"})
	require.NoError(t, err)
	assert.Equal(t, "status", statusCmd.Name())

	lsCmd, _, err := root.Find([]string{"ls"})
	require.NoError(t, err)
	assert.Equal(t, "ls", lsCmd.Name())
	assert.NotNil(t, lsCmd.Flags().Lookup("manual"))
	assert.NotNil(t, lsCmd.Flags().Lookup("auto"))
	assert.NotNil(t, lsCmd.Flags().Lookup("all"))
}

func TestStatusAndLsCommands_Offline(t *testing.T) {
	buf := new(bytes.Buffer)
	root := cmd.NewRootCommand()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"status", "--socket", "http://127.0.0.1:59999"})

	err := root.Execute()
	assert.Error(t, err)
}
