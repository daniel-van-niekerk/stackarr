package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// Client wraps the Docker client
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client
func NewClient() (*Client, error) {
	// Create Docker client with default settings
	// This connects to the Docker socket (usually /var/run/docker.sock)
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Close closes the Docker client connection
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping checks if Docker daemon is accessible
func (c *Client) Ping(ctx context.Context) error {
	// Create a timeout context
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Ping the Docker daemon
	_, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Docker daemon not accessible: %w", err)
	}

	return nil
}

// GetVersion gets Docker version information
func (c *Client) GetVersion(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	version, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get Docker version: %w", err)
	}

	return version.Version, nil
}

// IsDockerInstalled checks if Docker is installed and running
func IsDockerInstalled() bool {
	c, err := NewClient()
	if err != nil {
		log.Debug().Err(err).Msg("Docker client creation failed")
		return false
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		log.Debug().Err(err).Msg("Docker ping failed")
		return false
	}

	return true
}
