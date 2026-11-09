package dumper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// PostgresDumper handles full logical dumps for PostgreSQL / PGVector containers
type PostgresDumper struct {
	dockerClient *docker.Client
}

// NewPostgresDumper creates a new PostgresDumper
func NewPostgresDumper(dockerClient *docker.Client) *PostgresDumper {
	return &PostgresDumper{dockerClient: dockerClient}
}

// Dump executes pg_dumpall inside the container and streams output directly to zstd file
func (d *PostgresDumper) Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*DumpResult, error) {
	start := time.Now()

	// 1. Resolve postgres user from container inspect env
	user := "postgres"
	inspectRes, err := d.dockerClient.Engine().ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
	if err == nil && inspectRes.Container.Config != nil {
		for _, e := range inspectRes.Container.Config.Env {
			if strings.HasPrefix(e, "POSTGRES_USER=") {
				user = strings.TrimPrefix(e, "POSTGRES_USER=")
				break
			}
			if strings.HasPrefix(e, "PGUSER=") {
				user = strings.TrimPrefix(e, "PGUSER=")
				break
			}
		}
	}

	outFileName := "pg_dumpall.sql.zst"
	dstPath := filepath.Join(outDir, outFileName)

	// 2. Create exec process
	cmd := []string{"pg_dumpall", "-U", user}
	execCreate, err := d.dockerClient.Engine().ExecCreate(ctx, c.ID, client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pg_dumpall exec: %w", err)
	}

	// 3. Attach stream
	attachRes, err := d.dockerClient.Engine().ExecAttach(ctx, execCreate.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach pg_dumpall exec: %w", err)
	}
	defer attachRes.Close()

	// 4. Pipe stdout to zstd compress, collect stderr
	pr, pw := io.Pipe()
	stderrBuf := new(bytes.Buffer)

	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		_, copyErr := stdcopy.StdCopy(pw, stderrBuf, attachRes.Reader)
		errCh <- copyErr
	}()

	compressedSize, compErr := CompressToFile(pr, dstPath)
	copyErr := <-errCh

	if copyErr != nil {
		_ = os.Remove(dstPath)
		return nil, fmt.Errorf("error reading pg_dumpall stream: %w", copyErr)
	}
	if compErr != nil {
		_ = os.Remove(dstPath)
		return nil, fmt.Errorf("error compressing pg_dumpall: %w", compErr)
	}

	// 5. Inspect exit code
	execInspect, err := d.dockerClient.Engine().ExecInspect(ctx, execCreate.ID, client.ExecInspectOptions{})
	if err != nil {
		_ = os.Remove(dstPath)
		return nil, fmt.Errorf("failed to inspect exec exit status: %w", err)
	}
	if execInspect.ExitCode != 0 {
		_ = os.Remove(dstPath)
		errMsg := strings.TrimSpace(stderrBuf.String())
		if len(errMsg) > 300 {
			errMsg = errMsg[:300]
		}
		return nil, fmt.Errorf("pg_dumpall 失败 (退出码 %d): %s", execInspect.ExitCode, errMsg)
	}

	// 6. Minimum byte size sanity check (at least 1KB uncompressed equivalent)
	if compressedSize < 100 {
		_ = os.Remove(dstPath)
		return nil, fmt.Errorf("pg_dumpall 产物体积过小 (%d 字节)，疑似空库或导出异常", compressedSize)
	}

	duration := time.Since(start)
	return &DumpResult{
		Container:   c.Name,
		Kind:        "postgres",
		OutputDir:   outDir,
		Artifacts:   []string{outFileName},
		Description: fmt.Sprintf("pg_dumpall -U %s (全库与角色, zstd 流式压缩)", user),
		Duration:    duration,
		SizeBytes:   compressedSize,
	}, nil
}
