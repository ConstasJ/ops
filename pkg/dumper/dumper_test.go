package dumper_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ops-go/pkg/dumper"
)

func TestRotateBackups(t *testing.T) {
	tmpDir := t.TempDir()
	cName := "test-app"
	base := filepath.Join(tmpDir, cName)

	// Create 7 backup dirs with sequential timestamps
	for i := 1; i <= 7; i++ {
		dir := filepath.Join(base, time.Date(2026, 11, i, 10, 0, 0, 0, time.UTC).Format("20060102-150405"))
		require.NoError(t, os.MkdirAll(dir, 0700))
	}

	// Keep 5 -> oldest 2 should be deleted
	err := dumper.RotateBackups(tmpDir, cName, 5)
	require.NoError(t, err)

	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	assert.Equal(t, 5, len(entries))

	// Ensure oldest are gone (1st and 2nd) and newest remain (3rd to 7th)
	assert.Equal(t, "20261103-100000", entries[0].Name())
	assert.Equal(t, "20261107-100000", entries[4].Name())
}

func TestStreamZstdCompression(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "test.sql.zst")

	sampleData := bytes.Repeat([]byte("CREATE TABLE test (id INT, data TEXT);\nINSERT INTO test VALUES (1, 'demo');\n"), 100)
	inReader := bytes.NewReader(sampleData)

	size, err := dumper.CompressToFile(inReader, outFile)
	require.NoError(t, err)
	assert.True(t, size > 0)
	assert.True(t, size < int64(len(sampleData))) // compression ratio check

	// Decompress and verify
	compressedBytes, err := os.ReadFile(outFile)
	require.NoError(t, err)

	dec, err := zstd.NewReader(bytes.NewReader(compressedBytes))
	require.NoError(t, err)
	defer dec.Close()

	decompressed := new(bytes.Buffer)
	_, err = decompressed.ReadFrom(dec)
	require.NoError(t, err)

	assert.Equal(t, sampleData, decompressed.Bytes())
}

func TestWriteMetaJSON(t *testing.T) {
	tmpDir := t.TempDir()
	meta := dumper.MetaJSON{
		Container:     "test-db",
		At:            "2026-11-09T10:00:00",
		Kind:          "postgres",
		ImageBefore:   "pgvector:0.8.7",
		ImageIDBefore: "sha256:123456",
		What:          "pg_dumpall.sql.zst",
		SizeBytes:     102400,
		DurationSec:   1.5,
	}

	err := dumper.WriteMetaJSON(tmpDir, meta)
	require.NoError(t, err)

	metaPath := filepath.Join(tmpDir, "meta.json")
	assert.FileExists(t, metaPath)

	loaded, err := dumper.ReadMetaJSON(metaPath)
	require.NoError(t, err)
	assert.Equal(t, meta.Container, loaded.Container)
	assert.Equal(t, meta.What, loaded.What)
	assert.Equal(t, meta.SizeBytes, loaded.SizeBytes)
}
