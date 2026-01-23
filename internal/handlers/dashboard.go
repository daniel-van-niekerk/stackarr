package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/daniel-van-niekerk/stackarr/internal/services"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/rs/zerolog/log"
)

// AppVersion holds the application version, set by main
var AppVersion = "dev"

// DashboardHandlers holds dependencies for dashboard handlers
type DashboardHandlers struct {
	DB              *sql.DB
	Templates       *template.Template
	ProgressManager *streaming.ProgressManager
}

// ManagedContainer combines database container with Docker status
type ManagedContainer struct {
	ID           int64
	Name         string
	ServiceType  string
	Image        string
	IconURL      string
	Ports        []database.PortMapping
	DockerID     string
	Status       string // running, stopped, etc
	StatusDetail string // uptime, exit code, etc
	IsDeleted    bool   // true if container was deleted from Docker but still in database
}

// ShowDashboard displays the main dashboard
func (h *DashboardHandlers) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Get user info
	userID, _ := auth.GetUserSession(r)
	user, err := auth.GetUserByID(h.DB, userID)
	username := ""
	if err == nil && user != nil {
		username = user.Username
	}

	data := map[string]interface{}{
		"UserID":                 userID,
		"Username":               username,
		"DockerInstalled":        false,
		"ManagedContainers":      []ManagedContainer{},
		"ExternalContainers":     []docker.ContainerInfo{},
		"ShowExternalContainers": false,
		"ShowQuickStart":         true,
		"DarkMode":               false,
		"Version":                AppVersion,
	}

	// Get user preferences
	if user != nil {
		data["ShowExternalContainers"] = user.ShowExternalContainers
		data["ShowQuickStart"] = user.ShowQuickStart
		data["DarkMode"] = user.DarkMode
	}

	// Check Docker
	if !docker.IsDockerInstalled() {
		h.Templates.ExecuteTemplate(w, "dashboard.html", data)
		return
	}

	data["DockerInstalled"] = true

	// Get Docker client
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		h.Templates.ExecuteTemplate(w, "dashboard.html", data)
		return
	}
	defer client.Close()

	// Get Docker version
	version, err := client.GetVersion(ctx)
	if err == nil {
		data["DockerVersion"] = version
	}

	// Get all Docker containers
	allContainers, err := client.ListContainers(ctx, true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list containers")
		h.Templates.ExecuteTemplate(w, "dashboard.html", data)
		return
	}

	// Get managed containers from database
	dbContainers, err := (&database.DB{DB: h.DB}).ListContainers()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list database containers")
	}

	// Build map of Docker containers by ID
	dockerContainersMap := make(map[string]docker.ContainerInfo)
	for _, c := range allContainers {
		dockerContainersMap[c.ID] = c
	}

	// Build managed containers list from database
	managed := []ManagedContainer{}
	managedDockerIDs := make(map[string]bool)

	for _, dbContainer := range dbContainers {
		mc := ManagedContainer{
			ID:           dbContainer.ID,
			Name:         dbContainer.Name,
			ServiceType:  dbContainer.ServiceType,
			Image:        dbContainer.Image,
			IconURL:      dbContainer.IconURL,
			Ports:        dbContainer.Ports,
			DockerID:     dbContainer.DockerID,
			Status:       "stopped",
			StatusDetail: "Not running",
			IsDeleted:    false,
		}

		// If we have a DockerID, check if it's running
		if dbContainer.DockerID != "" {
			if dockerContainer, exists := dockerContainersMap[dbContainer.DockerID]; exists {
				mc.Status = dockerContainer.State
				mc.StatusDetail = dockerContainer.Status
				managedDockerIDs[dbContainer.DockerID] = true
			} else {
				// Container has a DockerID but doesn't exist in Docker - it was deleted
				mc.IsDeleted = true
				mc.Status = "deleted"
				mc.StatusDetail = "Container deleted from Docker"
			}
		}

		managed = append(managed, mc)
	}

	// Build external containers list (Docker containers not in database)
	external := []docker.ContainerInfo{}
	for _, dockerContainer := range allContainers {
		if !managedDockerIDs[dockerContainer.ID] {
			external = append(external, dockerContainer)
		}
	}

	data["ManagedContainers"] = managed
	data["ExternalContainers"] = external

	// Detect installed services efficiently using registry
	images := make([]string, 0, len(managed))
	for _, c := range managed {
		images = append(images, c.Image)
	}

	registry := services.NewRegistry()
	installed := registry.DetectInstalled(images)

	// Map installed services to data for template
	data["HasPlex"] = installed["plex"]
	data["HasSonarr"] = installed["sonarr"]
	data["HasRadarr"] = installed["radarr"]
	data["HasQBittorrent"] = installed["qbittorrent"]
	data["HasOverseerr"] = installed["overseerr"]
	data["HasSabnzbd"] = installed["sabnzbd"]
	data["HasProwlarr"] = installed["prowlarr"]
	data["HasBazarr"] = installed["bazarr"]
	data["HasFilebrowser"] = installed["filebrowser"]
	data["HasHomarr"] = installed["homarr"]

	h.Templates.ExecuteTemplate(w, "dashboard.html", data)
}

// ShowDockerInstall displays Docker installation instructions
// StartContainer handles starting a container
func (h *DashboardHandlers) StartContainer(w http.ResponseWriter, r *http.Request) {
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

	// Redirect back to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ToggleExternalContainers toggles the visibility of external containers
func (h *DashboardHandlers) ToggleExternalContainers(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserSession(r)

	// Get current user
	user, err := auth.GetUserByID(h.DB, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Toggle the preference
	newValue := !user.ShowExternalContainers
	if err := auth.UpdateUserPreference(h.DB, userID, newValue); err != nil {
		log.Error().Err(err).Msg("Failed to update preference")
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ToggleQuickStart toggles the visibility of the quickstart section
func (h *DashboardHandlers) ToggleQuickStart(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserSession(r)

	// Get current user
	user, err := auth.GetUserByID(h.DB, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Toggle the preference
	newValue := !user.ShowQuickStart
	if err := auth.UpdateQuickStartPreference(h.DB, userID, newValue); err != nil {
		log.Error().Err(err).Msg("Failed to update quickstart preference")
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ToggleDarkMode toggles the user's dark mode preference
func (h *DashboardHandlers) ToggleDarkMode(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserSession(r)

	// Get current user
	user, err := auth.GetUserByID(h.DB, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Toggle dark mode
	newValue := !user.DarkMode
	if err := auth.UpdateDarkMode(h.DB, userID, newValue); err != nil {
		log.Error().Err(err).Msg("Failed to update dark mode")
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// StopContainer handles stopping a container
func (h *DashboardHandlers) StopContainer(w http.ResponseWriter, r *http.Request) {
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

	// Redirect back to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// RestartContainer handles restarting a container
func (h *DashboardHandlers) RestartContainer(w http.ResponseWriter, r *http.Request) {
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

	// Redirect back to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// UpdateContainer handles updating a container to the latest image
func (h *DashboardHandlers) UpdateContainer(w http.ResponseWriter, r *http.Request) {
	// Get container ID from query parameter (this is the database ID)
	containerIDStr := r.URL.Query().Get("id")
	if containerIDStr == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	// Convert to int64
	var containerID int64
	if _, err := fmt.Sscanf(containerIDStr, "%d", &containerID); err != nil {
		http.Error(w, "Invalid container ID", http.StatusBadRequest)
		return
	}

	// Get container from database
	db := &database.DB{DB: h.DB}
	container, err := db.GetContainer(containerID)
	if err != nil {
		log.Error().Err(err).Int64("id", containerID).Msg("Failed to get container from database")
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	// Check if container has a DockerID
	if container.DockerID == "" {
		http.Error(w, "Container not created in Docker yet", http.StatusBadRequest)
		return
	}

	// Generate operation ID and launch async container update
	operationID := fmt.Sprintf("update-%s-%d", container.Name, time.Now().Unix())
	h.ProgressManager.CreateOperation(operationID)

	// Launch async operation
	go h.executeContainerUpdate(operationID, container)

	// Return operation ID as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"operation_id": operationID,
		"status":       "started",
	})
}

// executeContainerUpdate executes container update asynchronously with progress streaming
func (h *DashboardHandlers) executeContainerUpdate(operationID string, container *database.Container) {
	defer h.ProgressManager.Complete(operationID)

	ctx := context.Background()
	progressChan := make(chan streaming.ProgressEvent, 100)

	// Forward progress events to all subscribers
	go func() {
		for event := range progressChan {
			h.ProgressManager.Publish(operationID, event)
		}
	}()

	// Execute container update with progress
	log.Info().Str("operation_id", operationID).Str("name", container.Name).Msg("Starting async container update")

	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Failed to create Docker client")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Failed to connect to Docker: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		close(progressChan)
		return
	}
	defer client.Close()

	// Convert database port mappings to Docker port bindings
	portBindings := make([]docker.ContainerPortBinding, len(container.Ports))
	for i, p := range container.Ports {
		portBindings[i] = docker.ContainerPortBinding{
			ContainerPort: p.Container,
			HostPort:      p.Host,
			Protocol:      p.Protocol,
		}
	}

	// Convert volume mappings to bind strings
	volumes := make([]string, len(container.Volumes))
	for i, v := range container.Volumes {
		volumes[i] = fmt.Sprintf("%s:%s", v.Host, v.Container)
	}

	// Convert environment map to slice
	env := make([]string, 0, len(container.Environment))
	for k, v := range container.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Clear port bindings when using host mode (they're mutually exclusive)
	if container.NetworkMode == "host" {
		portBindings = nil
	}

	// Update the container with progress and network mode
	newDockerID, err := client.UpdateContainerWithProgressAndNetworkMode(ctx, container.DockerID, container.Name, container.Image, portBindings, volumes, env, container.NetworkMode, container.Privileged, progressChan)
	close(progressChan)

	if err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Str("name", container.Name).Msg("Container update failed")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Failed to update container: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Update the database with new DockerID
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "database_update",
		Message:   "Saving container to database...",
		Timestamp: time.Now().Format(time.RFC3339),
	})

	db := &database.DB{DB: h.DB}
	container.DockerID = newDockerID
	if err := db.UpdateContainer(container); err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Failed to update container DockerID in database")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Failed to save Docker ID: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Info().Str("operation_id", operationID).Str("name", container.Name).Str("new_docker_id", newDockerID).Msg("Container updated and saved successfully")

	// Send completion event
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "complete",
		Message:   "Container updated successfully",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"docker_id": newDockerID,
			"redirect":  "/dashboard",
		},
	})
}

// GetContainerLogs handles fetching container logs
func (h *DashboardHandlers) GetContainerLogs(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("id")
	if containerID == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	// Get tail parameter (default 500)
	tailStr := r.URL.Query().Get("tail")
	tail := 500
	if tailStr != "" {
		if parsedTail, err := strconv.Atoi(tailStr); err == nil && parsedTail > 0 {
			tail = parsedTail
		}
	}

	ctx := context.Background()
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}
	defer client.Close()

	logs, err := client.GetContainerLogs(ctx, containerID, tail)
	if err != nil {
		log.Error().Err(err).Str("container", containerID).Msg("Failed to fetch logs")
		http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	// Return logs as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":         logs,
		"container_id": containerID,
		"lines":        tail,
	})
}
