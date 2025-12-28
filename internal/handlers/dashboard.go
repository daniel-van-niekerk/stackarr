package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/rs/zerolog/log"
)

// AppVersion holds the application version, set by main
var AppVersion = "dev"

// DashboardHandlers holds dependencies for dashboard handlers
type DashboardHandlers struct {
	DB        *sql.DB
	Templates *template.Template
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
		"DarkMode":               false,
		"Version":                AppVersion,
	}

	// Get user preferences
	if user != nil {
		data["ShowExternalContainers"] = user.ShowExternalContainers
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

	// Check which pre-configured services are installed (based on image name)
	data["HasPlex"] = false
	data["HasSonarr"] = false
	data["HasRadarr"] = false
	data["HasQBittorrent"] = false
	data["HasOverseerr"] = false
	data["HasSabnzbd"] = false
	data["HasProwlarr"] = false
	data["HasBazarr"] = false
	data["HasFilebrowser"] = false
	data["HasHomarr"] = false

	for _, c := range managed {
		// Detect service type from image name
		if containsIgnoreCase(c.Image, "plex") {
			data["HasPlex"] = true
		}
		if containsIgnoreCase(c.Image, "sonarr") {
			data["HasSonarr"] = true
		}
		if containsIgnoreCase(c.Image, "radarr") {
			data["HasRadarr"] = true
		}
		if containsIgnoreCase(c.Image, "qbittorrent") {
			data["HasQBittorrent"] = true
		}
		if containsIgnoreCase(c.Image, "overseerr") {
			data["HasOverseerr"] = true
		}
		if containsIgnoreCase(c.Image, "sabnzbd") {
			data["HasSabnzbd"] = true
		}
		if containsIgnoreCase(c.Image, "prowlarr") {
			data["HasProwlarr"] = true
		}
		if containsIgnoreCase(c.Image, "bazarr") {
			data["HasBazarr"] = true
		}
		if containsIgnoreCase(c.Image, "filebrowser") {
			data["HasFilebrowser"] = true
		}
		if containsIgnoreCase(c.Image, "homarr") {
			data["HasHomarr"] = true
		}
	}

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

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
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

	ctx := context.Background()
	client, err := docker.NewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
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

	// Update the container
	log.Info().Str("name", container.Name).Str("image", container.Image).Msg("Updating container")
	newDockerID, err := client.UpdateContainer(ctx, container.DockerID, container.Name, container.Image, portBindings, volumes, env)
	if err != nil {
		log.Error().Err(err).Str("container", container.Name).Msg("Failed to update container")
		http.Error(w, fmt.Sprintf("Failed to update container: %v", err), http.StatusInternalServerError)
		return
	}

	// Update the DockerID in the database
	container.DockerID = newDockerID
	if err := db.UpdateContainer(container); err != nil {
		log.Error().Err(err).Int64("id", containerID).Str("docker_id", newDockerID).Msg("Failed to update container DockerID in database")
		// Continue anyway - the container is updated, just the database is out of sync
	}

	log.Info().Str("name", container.Name).Str("new_docker_id", newDockerID).Msg("Container updated successfully")

	// Redirect back to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
