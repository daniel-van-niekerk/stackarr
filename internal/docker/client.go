package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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

// PullImage pulls a Docker image
func (c *Client) PullImage(ctx context.Context, imageName string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	log.Info().Str("image", imageName).Msg("Pulling Docker image")
	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Read the pull output to completion (required for pull to actually happen)
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err != nil {
			break
		}
	}

	return nil
}

// ContainerPortBinding represents a port binding for container creation
type ContainerPortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

// CreateContainer creates a Docker container
func (c *Client) CreateContainer(ctx context.Context, name, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build port bindings
	portMap := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for _, pb := range portBindings {
		port, err := nat.NewPort(pb.Protocol, fmt.Sprintf("%d", pb.ContainerPort))
		if err != nil {
			return "", fmt.Errorf("invalid port: %w", err)
		}
		exposedPorts[port] = struct{}{}
		portMap[port] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", pb.HostPort),
			},
		}
	}

	// Create container config
	config := &container.Config{
		Image:        imageName,
		Env:          env,
		ExposedPorts: exposedPorts,
	}

	hostConfig := &container.HostConfig{
		PortBindings: portMap,
		Binds:        volumes,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	// Create the container
	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

// RemoveContainer removes a Docker container
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	log.Info().Str("id", containerID).Msg("Removing Docker container")

	options := container.RemoveOptions{
		Force: true,  // Force removal even if running
		RemoveVolumes: false,  // Don't remove volumes
	}

	if err := c.cli.ContainerRemove(ctx, containerID, options); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	return nil
}
