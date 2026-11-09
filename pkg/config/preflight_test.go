package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDirWritable_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	err := CheckDirWritable(tmpDir)
	assert.NoError(t, err)
}

func TestCheckFileWritable_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/test/state.json"
	err := CheckFileWritable(filePath)
	require.NoError(t, err)
}
