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

// UpdateContainer updates a container to the latest image version
// It pulls the latest image, stops the old container, removes it, and creates a new one with the same config
func (c *Client) UpdateContainer(ctx context.Context, containerID, containerName, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string) (string, error) {
	log.Info().Str("container", containerName).Str("image", imageName).Msg("Updating container to latest image")

	// Step 1: Pull the latest image
	log.Info().Str("image", imageName).Msg("Pulling latest image")
	if err := c.PullImage(ctx, imageName); err != nil {
		log.Warn().Err(err).Msg("Failed to pull image, will try with local image")
		// Continue anyway - maybe the local image is good enough
	}

	// Step 2: Stop the container if it's running
	log.Info().Str("container", containerID).Msg("Stopping container")
	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()

	timeout := 10
	if err := c.cli.ContainerStop(stopCtx, containerID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil {
		// Ignore error if container is already stopped
		log.Debug().Err(err).Msg("Container stop failed (may already be stopped)")
	}

	// Step 3: Remove the old container
	log.Info().Str("container", containerID).Msg("Removing old container")
	if err := c.RemoveContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("failed to remove old container: %w", err)
	}

	// Step 4: Create new container with the same configuration
	log.Info().Str("name", containerName).Msg("Creating new container")
	newContainerID, err := c.CreateContainer(ctx, containerName, imageName, portBindings, volumes, env)
	if err != nil {
		return "", fmt.Errorf("failed to create new container (old container removed): %w", err)
	}

	// Step 5: Start the new container
	log.Info().Str("container", newContainerID).Msg("Starting new container")
	startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
	defer startCancel()

	if err := c.cli.ContainerStart(startCtx, newContainerID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start new container (old container removed): %w", err)
	}

	log.Info().Str("old_id", containerID).Str("new_id", newContainerID).Msg("Container updated successfully")
	return newContainerID, nil
}
