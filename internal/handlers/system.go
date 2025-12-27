package handlers

import (
	"context"
	"html/template"
	"net/http"

	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/rs/zerolog/log"
)

// SystemHandlers holds dependencies for system handlers
type SystemHandlers struct {
	Templates *template.Template
}

// ShowSystemStatus displays Docker and system status
func (h *SystemHandlers) ShowSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	data := map[string]interface{}{
		"DockerInstalled": false,
	}

	// Check if Docker is installed
	if !docker.IsDockerInstalled() {
		log.Warn().Msg("Docker is not installed or not running")
		http.Redirect(w, r, "/docker-install", http.StatusSeeOther)
		return
	}

	data["DockerInstalled"] = true

	// Create Docker client
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		h.Templates.ExecuteTemplate(w, "system.html", data)
		return
	}
	defer client.Close()

	// Get Docker version
	version, err := client.GetVersion(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get Docker version")
	} else {
		data["DockerVersion"] = version
	}

	// List containers (true = include stopped containers)
	containers, err := client.ListContainers(ctx, true) // All containers
	if err != nil {
		log.Error().Err(err).Msg("Failed to list containers")
	} else {
		data["Containers"] = containers
	}

	h.Templates.ExecuteTemplate(w, "system.html", data)
}

// ShowDockerInstall displays Docker installation instructions
func (h *SystemHandlers) ShowDockerInstall(w http.ResponseWriter, r *http.Request) {
	h.Templates.ExecuteTemplate(w, "docker-install.html", nil)
}

// StartContainer handles starting a container
func (h *SystemHandlers) StartContainer(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("id")
	if containerID == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if err := client.StartContainer(ctx, containerID); err != nil {
		log.Error().Err(err).Str("container", containerID).Msg("Failed to start container")
		http.Error(w, "Failed to start container", http.StatusInternalServerError)
		return
	}

	// Redirect back to system page
	http.Redirect(w, r, "/system", http.StatusSeeOther)
}

// StopContainer handles stopping a container
func (h *SystemHandlers) StopContainer(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("id")
	if containerID == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if err := client.StopContainer(ctx, containerID); err != nil {
		log.Error().Err(err).Str("container", containerID).Msg("Failed to stop container")
		http.Error(w, "Failed to stop container", http.StatusInternalServerError)
		return
	}

	// Redirect back to system page
	http.Redirect(w, r, "/system", http.StatusSeeOther)
}

// RestartContainer handles restarting a container
func (h *SystemHandlers) RestartContainer(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("id")
	if containerID == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if err := client.RestartContainer(ctx, containerID); err != nil {
		log.Error().Err(err).Str("container", containerID).Msg("Failed to restart container")
		http.Error(w, "Failed to restart container", http.StatusInternalServerError)
		return
	}

	// Redirect back to system page
	http.Redirect(w, r, "/system", http.StatusSeeOther)
}
