package docker

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/moby/moby/client"
)

// Client encapsulates both low-level Docker Engine Client and high-level Compose v5 Service
type Client struct {
	engine  *client.Client
	cli     *command.DockerCli
	compose api.Compose
}

// NewClient initializes a dual-channel Docker and Compose client
func NewClient(ctx context.Context, host string) (*Client, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	if host != "" {
		opts = append(opts, client.WithHost(host))
	} else if os.Getenv("DOCKER_HOST") != "" {
		opts = append(opts, client.FromEnv)
	} else {
		// Use standard default host socket
		opts = append(opts, client.WithHost(client.DefaultDockerHost))
	}

	engineClient, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker engine client: %w", err)
	}

	// Initialize Docker CLI for Compose SDK
	dockerCli, err := command.NewDockerCli(command.WithCombinedStreams(os.Stdout))
	if err != nil {
		_ = engineClient.Close()
		return nil, fmt.Errorf("failed to create docker cli wrapper: %w", err)
	}

	cliOpts := flags.NewClientOptions()
	if host != "" {
		cliOpts.Hosts = []string{host}
	}
	if err := dockerCli.Initialize(cliOpts); err != nil {
		_ = engineClient.Close()
		return nil, fmt.Errorf("failed to initialize docker cli wrapper: %w", err)
	}

	composeService, err := compose.NewComposeService(dockerCli)
	if err != nil {
		_ = engineClient.Close()
		return nil, fmt.Errorf("failed to create compose service: %w", err)
	}

	return &Client{
		engine:  engineClient,
		cli:     dockerCli,
		compose: composeService,
	}, nil
}

// Ping checks if Docker daemon is reachable
func (c *Client) Ping(ctx context.Context) (client.PingResult, error) {
	return c.engine.Ping(ctx, client.PingOptions{})
}

// ServerVersion returns the Docker Engine server version details
func (c *Client) ServerVersion(ctx context.Context) (client.ServerVersionResult, error) {
	return c.engine.ServerVersion(ctx, client.ServerVersionOptions{})
}

// Engine returns the underlying Moby engine client
func (c *Client) Engine() *client.Client {
	return c.engine
}

// Compose returns the Compose v5 service
func (c *Client) Compose() api.Compose {
	return c.compose
}

// Close closes the underlying engine client
func (c *Client) Close() error {
	if c.engine != nil {
		return c.engine.Close()
	}
	return nil
}
