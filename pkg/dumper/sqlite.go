package dumper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// SQLiteDumper handles SQLite database backups
type SQLiteDumper struct {
	dockerClient *docker.Client
}

// NewSQLiteDumper creates a new SQLiteDumper
func NewSQLiteDumper(dockerClient *docker.Client) *SQLiteDumper {
	return &SQLiteDumper{dockerClient: dockerClient}
}

// Dump locates sqlite databases and creates a compressed snapshot
func (d *SQLiteDumper) Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*DumpResult, error) {
	start := time.Now()

	// Default candidates based on known container paths
	cands := []string{
		"/data/db.sqlite3",
		"/data/db.sqlite",
		"/usr/local/headscale/data/db.sqlite",
		"/var/lib/headscale/db.sqlite",
		"/data",
	}

	var foundPath string
	for _, cand := range cands {
		res, err := d.dockerClient.Engine().CopyFromContainer(ctx, c.ID, client.CopyFromContainerOptions{
			SourcePath: cand,
		})
		if err == nil {
			_ = res.Content.Close()
			foundPath = cand
			break
		}
	}

	if foundPath == "" {
		// Fallback: check bind mounts on host
		inspectRes, err := d.dockerClient.Engine().ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		if err == nil {
			for _, m := range inspectRes.Container.Mounts {
				if m.Type == "bind" {
					_ = filepath.Walk(m.Source, func(path string, info os.FileInfo, err error) error {
						if err == nil && !info.IsDir() && strings.HasSuffix(path, ".sqlite3") {
							foundPath = m.Destination + "/" + filepath.Base(path)
						}
						return nil
					})
				}
			}
		}
	}

	if foundPath == "" {
		return nil, fmt.Errorf("未在容器 %s 内找到常见的 sqlite 数据库文件", c.Name)
	}

	// Copy tar stream of the database file
	copyRes, err := d.dockerClient.Engine().CopyFromContainer(ctx, c.ID, client.CopyFromContainerOptions{
		SourcePath: foundPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to extract sqlite file %s: %w", foundPath, err)
	}
	defer copyRes.Content.Close()

	outFileName := "sqlite_db.tar.zst"
	dstPath := filepath.Join(outDir, outFileName)

	size, err := CompressToFile(copyRes.Content, dstPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compress sqlite db: %w", err)
	}

	return &DumpResult{
		Container:   c.Name,
		Kind:        "sqlite",
		OutputDir:   outDir,
		Artifacts:   []string{outFileName},
		Description: fmt.Sprintf("SQLite 文件快照 (%s, zstd 归档压缩)", foundPath),
		Duration:    time.Since(start),
		SizeBytes:   size,
	}, nil
}
