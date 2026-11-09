package cmd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestPendingCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()

	pendingCmd, _, err := root.Find([]string{"pending"})
	require.NoError(t, err)
	assert.Equal(t, "pending", pendingCmd.Name())
	assert.NotNil(t, pendingCmd.Flags().Lookup("all"))
	assert.NotNil(t, pendingCmd.Flags().Lookup("concurrency"))
}

func TestPendingCommand_Offline(t *testing.T) {
	buf := new(bytes.Buffer)
	root := cmd.NewRootCommand()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"pending", "--socket", "http://127.0.0.1:59999"})

	err := root.Execute()
	assert.Error(t, err)
}
