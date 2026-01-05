package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
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

// DockerPullProgress represents a Docker pull progress response
type DockerPullProgress struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Progress       string `json:"progress,omitempty"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail,omitempty"`
	Error string `json:"error,omitempty"`
}

// PullImageWithProgress pulls a Docker image and streams progress events
func (c *Client) PullImageWithProgress(ctx context.Context, imageName string, progressChan chan<- streaming.ProgressEvent) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	log.Info().Str("image", imageName).Msg("Pulling Docker image with progress")

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Parse and stream progress
	decoder := json.NewDecoder(reader)
	layerProgress := make(map[string]*streaming.LayerProgress) // Track each layer

	for {
		var progress DockerPullProgress
		if err := decoder.Decode(&progress); err != nil {
			if err == io.EOF {
				break
			}
			log.Warn().Err(err).Msg("Error decoding progress")
			continue
		}

		// Handle errors
		if progress.Error != "" {
			progressChan <- streaming.ProgressEvent{
				Type:      "error",
				Message:   progress.Error,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			return fmt.Errorf("image pull error: %s", progress.Error)
		}

		// Send progress updates
		if progress.ID != "" {
			layer := parseLayerProgress(progress, layerProgress)
			// Truncate layer ID to 12 characters if longer, otherwise use full ID
			layerIDDisplay := progress.ID
			if len(progress.ID) > 12 {
				layerIDDisplay = progress.ID[:12]
			}
			progressChan <- streaming.ProgressEvent{
				Type:      "layer_download",
				Message:   fmt.Sprintf("Layer %s: %s", layerIDDisplay, progress.Status),
				Timestamp: time.Now().Format(time.RFC3339),
				Data:      layer,
			}
		} else {
			// Overall status messages
			progressChan <- streaming.ProgressEvent{
				Type:      "image_pull",
				Message:   progress.Status,
				Timestamp: time.Now().Format(time.RFC3339),
			}
		}
	}

	return nil
}

// parseLayerProgress parses and updates layer progress information
func parseLayerProgress(dockerProgress DockerPullProgress, layerMap map[string]*streaming.LayerProgress) *streaming.LayerProgress {
	layerID := dockerProgress.ID
	if layerMap[layerID] == nil {
		layerMap[layerID] = &streaming.LayerProgress{LayerID: layerID}
	}

	layer := layerMap[layerID]
	layer.Status = dockerProgress.Status
	layer.Current = dockerProgress.ProgressDetail.Current
	layer.Total = dockerProgress.ProgressDetail.Total

	if layer.Total > 0 {
		layer.Progress = int((layer.Current * 100) / layer.Total)
	}

	return layer
}

// ContainerPortBinding represents a port binding for container creation
type ContainerPortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

// CreateContainer creates a Docker container
func (c *Client) CreateContainer(ctx context.Context, name, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string) (string, error) {
	return c.CreateContainerWithNetworkMode(ctx, name, imageName, portBindings, volumes, env, "")
}

// CreateContainerWithNetworkMode creates a Docker container with optional network mode
func (c *Client) CreateContainerWithNetworkMode(ctx context.Context, name, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string, networkMode string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build port bindings only if not using host networking (they're mutually exclusive)
	portMap := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	if networkMode != "host" {
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

	// Set network mode if specified
	if networkMode != "" {
		hostConfig.NetworkMode = container.NetworkMode(networkMode)
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
	return c.UpdateContainerWithNetworkMode(ctx, containerID, containerName, imageName, portBindings, volumes, env, "")
}

// UpdateContainerWithNetworkMode updates a container with optional network mode
func (c *Client) UpdateContainerWithNetworkMode(ctx context.Context, containerID, containerName, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string, networkMode string) (string, error) {
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
	newContainerID, err := c.CreateContainerWithNetworkMode(ctx, containerName, imageName, portBindings, volumes, env, networkMode)
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

// UpdateContainerWithProgress updates a container to the latest image version with progress streaming
func (c *Client) UpdateContainerWithProgress(ctx context.Context, containerID, containerName, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string, progressChan chan<- streaming.ProgressEvent) (string, error) {
	return c.UpdateContainerWithProgressAndNetworkMode(ctx, containerID, containerName, imageName, portBindings, volumes, env, "", progressChan)
}

// UpdateContainerWithProgressAndNetworkMode updates a container with optional network mode and progress streaming
func (c *Client) UpdateContainerWithProgressAndNetworkMode(ctx context.Context, containerID, containerName, imageName string, portBindings []ContainerPortBinding, volumes []string, env []string, networkMode string, progressChan chan<- streaming.ProgressEvent) (string, error) {
	log.Info().Str("container", containerName).Str("image", imageName).Msg("Updating container to latest image with progress")

	// Step 1: Pull the latest image with progress
	progressChan <- streaming.ProgressEvent{
		Type:      "image_pull",
		Message:   fmt.Sprintf("Pulling latest image: %s", imageName),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := c.PullImageWithProgress(ctx, imageName, progressChan); err != nil {
		log.Warn().Err(err).Msg("Failed to pull image, will try with local image")
		// Continue anyway - maybe the local image is good enough
	}

	// Step 2: Stop the container if it's running
	progressChan <- streaming.ProgressEvent{
		Type:      "container_stop",
		Message:   "Stopping old container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

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
	progressChan <- streaming.ProgressEvent{
		Type:      "container_remove",
		Message:   "Removing old container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := c.RemoveContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("failed to remove old container: %w", err)
	}

	// Step 4: Create new container with the same configuration
	progressChan <- streaming.ProgressEvent{
		Type:      "container_create",
		Message:   "Creating new container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	newContainerID, err := c.CreateContainerWithNetworkMode(ctx, containerName, imageName, portBindings, volumes, env, networkMode)
	if err != nil {
		return "", fmt.Errorf("failed to create new container (old container removed): %w", err)
	}

	// Step 5: Start the new container
	progressChan <- streaming.ProgressEvent{
		Type:      "container_start",
		Message:   "Starting new container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
	defer startCancel()

	if err := c.cli.ContainerStart(startCtx, newContainerID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start new container (old container removed): %w", err)
	}

	log.Info().Str("old_id", containerID).Str("new_id", newContainerID).Msg("Container updated successfully")
	return newContainerID, nil
}
