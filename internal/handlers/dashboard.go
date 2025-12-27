package handlers

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/rs/zerolog/log"
)

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
	DockerID     string
	Status       string // running, stopped, etc
	StatusDetail string // uptime, exit code, etc
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
	}

	// Get user preference for showing external containers
	if user != nil {
		data["ShowExternalContainers"] = user.ShowExternalContainers
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

	// Build map of managed container Docker IDs
	managedIDs := make(map[string]*database.Container)
	for _, c := range dbContainers {
		if c.DockerID != "" {
			managedIDs[c.DockerID] = c
		}
	}

	// Separate managed and external containers
	managed := []ManagedContainer{}
	external := []docker.ContainerInfo{}

	for _, dockerContainer := range allContainers {
		if dbContainer, exists := managedIDs[dockerContainer.ID]; exists {
			// This is a managed container
			managed = append(managed, ManagedContainer{
				ID:           dbContainer.ID,
				Name:         dbContainer.Name,
				ServiceType:  dbContainer.ServiceType,
				Image:        dbContainer.Image,
				DockerID:     dockerContainer.ID,
				Status:       dockerContainer.State,
				StatusDetail: dockerContainer.Status,
			})
		} else {
			// This is an external container
			external = append(external, dockerContainer)
		}
	}

	data["ManagedContainers"] = managed
	data["ExternalContainers"] = external

	// Check which pre-configured services are installed
	data["HasPlex"] = false
	data["HasSonarr"] = false
	data["HasRadarr"] = false
	data["HasQBittorrent"] = false

	for _, c := range managed {
		switch c.ServiceType {
		case "plex":
			data["HasPlex"] = true
		case "sonarr":
			data["HasSonarr"] = true
		case "radarr":
			data["HasRadarr"] = true
		case "qbittorrent":
			data["HasQBittorrent"] = true
		}
	}

	h.Templates.ExecuteTemplate(w, "dashboard.html", data)
}

// ShowDockerInstall displays Docker installation instructions
func (h *DashboardHandlers) ShowDockerInstall(w http.ResponseWriter, r *http.Request) {
	h.Templates.ExecuteTemplate(w, "docker-install.html", nil)
}

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
