package dumper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// DumpResult records outcome of a snapshot
type DumpResult struct {
	Container   string
	Kind        string
	OutputDir   string
	Artifacts   []string
	Description string
	Duration    time.Duration
	SizeBytes   int64
}

// MetaJSON represents metadata stored in meta.json
type MetaJSON struct {
	Container     string  `json:"container"`
	At            string  `json:"at"`
	Kind          string  `json:"kind"`
	ImageBefore   string  `json:"image_before"`
	ImageIDBefore string  `json:"image_id_before"`
	What          string  `json:"what"`
	DurationSec   float64 `json:"duration_sec,omitempty"`
	SizeBytes     int64   `json:"size_bytes,omitempty"`
}

// Dumper is the storage backup interface
type Dumper interface {
	Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*DumpResult, error)
}

// CreateBackupDir creates a timestamped subdirectory under baseRoot/containerName
func CreateBackupDir(baseRoot, containerName string) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(baseRoot, containerName, stamp)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}
	return dir, nil
}

// RotateBackups keeps the most recent `keep` directories and removes older ones
func RotateBackups(baseRoot, containerName string, keep int) error {
	if keep <= 0 {
		keep = 5
	}
	root := filepath.Join(baseRoot, containerName)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}

	if len(dirs) <= keep {
		return nil
	}

	sort.Strings(dirs)
	toRemove := dirs[:len(dirs)-keep]
	for _, old := range toRemove {
		_ = os.RemoveAll(filepath.Join(root, old))
	}
	return nil
}

// CompressToFile streams input through zstd writer directly into target file
func CompressToFile(r io.Reader, dstPath string) (int64, error) {
	f, err := os.Create(dstPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file %s: %w", dstPath, err)
	}
	defer f.Close()

	enc, err := zstd.NewWriter(f)
	if err != nil {
		return 0, fmt.Errorf("failed to create zstd writer: %w", err)
	}

	written, err := io.Copy(enc, r)
	if err != nil {
		_ = enc.Close()
		return 0, fmt.Errorf("compression streaming failed: %w", err)
	}

	if err := enc.Close(); err != nil {
		return 0, fmt.Errorf("failed to close zstd writer: %w", err)
	}

	info, err := f.Stat()
	if err == nil {
		return info.Size(), nil
	}
	return written, nil
}

// WriteMetaJSON writes meta.json into dir
func WriteMetaJSON(dir string, meta MetaJSON) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0644)
}

// ReadMetaJSON reads meta.json from path
func ReadMetaJSON(path string) (*MetaJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta MetaJSON
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// GetDumper returns the appropriate dumper implementation for a given container kind
func GetDumper(kind string, dockerClient *docker.Client) (Dumper, error) {
	switch strings.ToLower(kind) {
	case "postgres":
		return NewPostgresDumper(dockerClient), nil
	case "redis":
		return NewRedisDumper(dockerClient), nil
	case "sqlite":
		return NewSQLiteDumper(dockerClient), nil
	case "gitea":
		return NewGiteaDumper(dockerClient), nil
	default:
		return nil, fmt.Errorf("unsupported or stateless storage kind: %q", kind)
	}
}
