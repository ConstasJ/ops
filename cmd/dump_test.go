package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestDumpCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()
	dumpCmd, _, err := root.Find([]string{"dump"})
	require.NoError(t, err)
	assert.Equal(t, "dump", dumpCmd.Name())
	assert.NotNil(t, dumpCmd.Flags().Lookup("full"))
	assert.NotNil(t, dumpCmd.Flags().Lookup("backup-root"))
}
