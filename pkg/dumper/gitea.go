package dumper

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"ops-go/pkg/collector"
	"ops-go/pkg/docker"
)

// GiteaDumper handles Gitea database and full repo dumps
type GiteaDumper struct {
	dockerClient *docker.Client
	Full         bool
}

// NewGiteaDumper creates a new GiteaDumper
func NewGiteaDumper(dockerClient *docker.Client) *GiteaDumper {
	return &GiteaDumper{dockerClient: dockerClient}
}

// Dump executes /usr/local/bin/gitea dump inside the container and copies output out
func (d *GiteaDumper) Dump(ctx context.Context, c *collector.ContainerInfo, outDir string) (*DumpResult, error) {
	start := time.Now()

	tmpZip := fmt.Sprintf("/tmp/ops-gitea-%d.zip", time.Now().Unix())
	cmd := []string{
		"/usr/local/bin/gitea", "dump",
		"-c", "/data/gitea/conf/app.ini",
		"-f", tmpZip,
	}

	desc := "gitea dump --full (全量打包：数据库、仓库、LFS与附件)"
	if !d.Full {
		cmd = append(cmd,
			"-R",
			"--skip-log",
			"--skip-attachment-data",
			"--skip-lfs-data",
			"--skip-package-data",
			"--skip-index",
			"--skip-custom-dir",
		)
		desc = "gitea dump (仅数据库与配置，跳过仓库/LFS/附件/索引)"
	}

	// 1. Run gitea dump as git user
	execCreate, err := d.dockerClient.Engine().ExecCreate(ctx, c.ID, client.ExecCreateOptions{
		User:         "git",
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create gitea dump exec: %w", err)
	}

	attachRes, err := d.dockerClient.Engine().ExecAttach(ctx, execCreate.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach gitea exec: %w", err)
	}
	defer attachRes.Close()

	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	_, _ = stdcopy.StdCopy(stdoutBuf, stderrBuf, attachRes.Reader)

	inspect, err := d.dockerClient.Engine().ExecInspect(ctx, execCreate.ID, client.ExecInspectOptions{})
	if err != nil || inspect.ExitCode != 0 {
		return nil, fmt.Errorf("gitea dump 失败: %s", stderrBuf.String())
	}

	// 2. Copy zip from container
	copyRes, err := d.dockerClient.Engine().CopyFromContainer(ctx, c.ID, client.CopyFromContainerOptions{
		SourcePath: tmpZip,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to copy gitea zip from container: %w", err)
	}
	defer copyRes.Content.Close()

	outFileName := "gitea-dump.zip"
	if !d.Full {
		outFileName = "gitea-db-only.zip"
	}
	dstPath := filepath.Join(outDir, outFileName)

	// Docker CopyFromContainer returns a tar stream containing the file
	tarReader := tar.NewReader(copyRes.Content)
	var written int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading gitea tar stream: %w", err)
		}
		if header.Typeflag == tar.TypeReg {
			f, err := os.Create(dstPath)
			if err != nil {
				return nil, err
			}
			written, err = io.Copy(f, tarReader)
			_ = f.Close()
			if err != nil {
				return nil, err
			}
			break
		}
	}

	// Clean up zip inside container
	cleanupExec, _ := d.dockerClient.Engine().ExecCreate(ctx, c.ID, client.ExecCreateOptions{
		Cmd: []string{"rm", "-f", tmpZip},
	})
	if cleanupExec.ID != "" {
		_, _ = d.dockerClient.Engine().ExecStart(ctx, cleanupExec.ID, client.ExecStartOptions{})
	}

	return &DumpResult{
		Container:   c.Name,
		Kind:        "gitea",
		OutputDir:   outDir,
		Artifacts:   []string{outFileName},
		Description: desc,
		Duration:    time.Since(start),
		SizeBytes:   written,
	}, nil
}
