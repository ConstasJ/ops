package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/cmd"
)

func TestDoctorCommand_Registered(t *testing.T) {
	root := cmd.NewRootCommand()
	docCmd, _, err := root.Find([]string{"doctor"})
	require.NoError(t, err)
	assert.Equal(t, "doctor", docCmd.Name())
	assert.NotNil(t, docCmd.Flags().Lookup("backup-root"))
}
