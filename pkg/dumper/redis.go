package dumper

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// RedisDumper handles Redis RDB & AOF backups
type RedisDumper struct {
	dockerClient *docker.Client
}

// NewRedisDumper creates a new RedisDumper
func NewRedisDumper(dockerClient *docker.Client) *RedisDumper {
	return &RedisDumper{dockerClient: dockerClient}
}

// Dump runs redis-cli SAVE and archives /data
func (d *RedisDumper) Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*DumpResult, error) {
	start := time.Now()

	// 1. Run redis-cli SAVE
	execCreate, err := d.dockerClient.Engine().ExecCreate(ctx, c.ID, client.ExecCreateOptions{
		Cmd:          []string{"redis-cli", "SAVE"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create redis-cli SAVE exec: %w", err)
	}

	attachRes, err := d.dockerClient.Engine().ExecAttach(ctx, execCreate.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach redis exec: %w", err)
	}
	defer attachRes.Close()

	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	_, _ = stdcopy.StdCopy(stdoutBuf, stderrBuf, attachRes.Reader)

	if !strings.Contains(stdoutBuf.String(), "OK") {
		return nil, fmt.Errorf("redis SAVE 执行未成功: stdout=%q, stderr=%q", stdoutBuf.String(), stderrBuf.String())
	}

	// 2. Stream /data tar archive from container directly to zstd compressed file
	copyRes, err := d.dockerClient.Engine().CopyFromContainer(ctx, c.ID, client.CopyFromContainerOptions{
		SourcePath: "/data",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy /data from redis container: %w", err)
	}
	defer copyRes.Content.Close()

	outFileName := "redis_data.tar.zst"
	dstPath := filepath.Join(outDir, outFileName)

	compressedSize, err := CompressToFile(copyRes.Content, dstPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compress redis data: %w", err)
	}

	return &DumpResult{
		Container:   c.Name,
		Kind:        "redis",
		OutputDir:   outDir,
		Artifacts:   []string{outFileName},
		Description: "redis-cli SAVE + /data (RDB/AOF, zstd 归档压缩)",
		Duration:    time.Since(start),
		SizeBytes:   compressedSize,
	}, nil
}
