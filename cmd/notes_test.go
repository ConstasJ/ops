package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestNotesCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()
	notesCmd, _, err := root.Find([]string{"notes"})
	require.NoError(t, err)
	assert.Equal(t, "notes", notesCmd.Name())
}
