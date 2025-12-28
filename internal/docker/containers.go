package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
)

// ContainerInfo holds basic container information
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	Created time.Time
	Ports   []PortBinding
}

// PortBinding represents a port mapping
type PortBinding struct {
	HostPort      string
	ContainerPort string
	Protocol      string
}

// ListContainers lists all containers (running and stopped)
func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// List containers
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All: all, // true = include stopped containers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Convert to our ContainerInfo type
	result := make([]ContainerInfo, 0, len(containers))
	for _, cont := range containers {
		// Container names start with /, remove it
		name := cont.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}

		// Parse port bindings
		ports := make([]PortBinding, 0)
		for _, port := range cont.Ports {
			if port.PublicPort > 0 {
				ports = append(ports, PortBinding{
					HostPort:      fmt.Sprintf("%d", port.PublicPort),
					ContainerPort: fmt.Sprintf("%d", port.PrivatePort),
					Protocol:      port.Type,
				})
			}
		}

		result = append(result, ContainerInfo{
			ID:      cont.ID, // Full container ID
			Name:    name,
			Image:   cont.Image,
			State:   cont.State,
			Status:  cont.Status,
			Created: time.Unix(cont.Created, 0),
			Ports:   ports,
		})
	}

	log.Debug().Int("count", len(result)).Msg("Listed containers")
	return result, nil
}

// StartContainer starts a container by ID or name
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	log.Info().Str("container", containerID).Msg("Container started")
	return nil
}

// StopContainer stops a container by ID or name
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Stop with 10 second timeout for graceful shutdown
	timeout := 10
	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	log.Info().Str("container", containerID).Msg("Container stopped")
	return nil
}

// RestartContainer restarts a container by ID or name
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	timeout := 10
	if err := c.cli.ContainerRestart(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	}); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}

	log.Info().Str("container", containerID).Msg("Container restarted")
	return nil
}

// GetContainerStatus gets the current status of a container
func (c *Client) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	inspect, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	return inspect.State.Status, nil
}
