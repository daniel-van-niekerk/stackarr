package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/rs/zerolog/log"
)

// executeContainerCreate executes container creation asynchronously with progress streaming
func (h *ContainerHandlers) executeContainerCreate(operationID string, data ContainerFormData) {
	defer h.ProgressManager.Complete(operationID)

	ctx := context.Background()
	progressChan := make(chan streaming.ProgressEvent, 100)

	// Forward progress events to all subscribers
	go func() {
		for event := range progressChan {
			h.ProgressManager.Publish(operationID, event)
		}
	}()

	// Execute container creation with progress
	log.Info().Str("operation_id", operationID).Str("name", data.Name).Msg("Starting async container create")

	dockerID, err := h.createAndStartContainerWithProgress(ctx, data, progressChan)
	close(progressChan)

	if err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Container creation failed")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Failed to create container: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Update database with Docker ID
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "database_update",
		Message:   "Saving container to database...",
		Timestamp: time.Now().Format(time.RFC3339),
	})

	db := &database.DB{DB: h.DB}
	container := &database.Container{
		ID:          data.ID,
		Name:        data.Name,
		ServiceType: "",
		Image:       data.Image,
		IconURL:     data.IconURL,
		NetworkMode: data.NetworkMode,
		Privileged:  data.Privileged,
		Ports:       data.PortMappings,
		Volumes:     data.VolumeMappings,
		Environment: data.EnvVars,
		DockerID:    dockerID,
		Enabled:     true,
	}

	if err := db.UpdateContainer(container); err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Failed to update container with Docker ID")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Failed to save Docker ID: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Info().Str("operation_id", operationID).Str("docker_id", dockerID).Msg("Container created and saved successfully")

	// Send completion event
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "complete",
		Message:   "Container created successfully",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"docker_id": dockerID,
			"redirect":  "/dashboard",
		},
	})
}

// createAndStartContainerWithProgress creates and starts a Docker container with progress streaming
func (h *ContainerHandlers) createAndStartContainerWithProgress(ctx context.Context, data ContainerFormData, progressChan chan<- streaming.ProgressEvent) (string, error) {
	client, err := docker.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer client.Close()

	// Pull the image with progress
	progressChan <- streaming.ProgressEvent{
		Type:      "image_pull",
		Message:   fmt.Sprintf("Pulling image %s...", data.Image),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := client.PullImageWithProgress(ctx, data.Image, progressChan); err != nil {
		log.Warn().Err(err).Msg("Failed to pull image, will try to use local image")
		// Continue anyway - maybe the local image is good enough
	}

	// Convert ports to Docker format
	portBindings := make([]docker.ContainerPortBinding, 0, len(data.PortMappings))
	for _, p := range data.PortMappings {
		portBindings = append(portBindings, docker.ContainerPortBinding{
			ContainerPort: p.Container,
			HostPort:      p.Host,
			Protocol:      p.Protocol,
		})
	}

	// Convert volumes to Docker format
	volumeBindings := make([]string, 0, len(data.VolumeMappings))
	for _, v := range data.VolumeMappings {
		volumeBindings = append(volumeBindings, fmt.Sprintf("%s:%s", v.Host, v.Container))
	}

	// Create host directories for volume mounts
	if err := docker.CreateVolumeDirectories(data.VolumeMappings); err != nil {
		log.Warn().Err(err).Msg("Failed to create volume directories")
		// Continue anyway - some volumes might be Docker volumes or already exist
	}

	// Convert env vars to Docker format
	envList := make([]string, 0, len(data.EnvVars))
	for k, v := range data.EnvVars {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Create the container
	progressChan <- streaming.ProgressEvent{
		Type:      "container_create",
		Message:   "Creating Docker container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Use network mode from container data
	networkMode := data.NetworkMode
	// Clear port bindings when using host mode (they're mutually exclusive)
	if networkMode == "host" {
		portBindings = nil
	}

	containerID, err := client.CreateContainerWithNetworkMode(ctx, data.Name, data.Image, portBindings, volumeBindings, envList, networkMode, data.Privileged)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	progressChan <- streaming.ProgressEvent{
		Type:      "container_start",
		Message:   "Starting Docker container...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := client.StartContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return containerID, nil
}
