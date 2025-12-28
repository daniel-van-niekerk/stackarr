package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/rs/zerolog/log"
)

// ContainerHandlers holds dependencies for container handlers
type ContainerHandlers struct {
	DB        *sql.DB
	Templates *template.Template
}

// ContainerFormData represents the container form data
type ContainerFormData struct {
	ID             int64
	Name           string
	Image          string
	IconURL        string
	PortMappings   []database.PortMapping
	VolumeMappings []database.VolumeMapping
	EnvVars        map[string]string
}

// ServicePreview represents service preview information
type ServicePreview struct {
	Name        string
	Description string
	IconURL     string
}

// ServiceTemplate represents a pre-configured service template
type ServiceTemplate struct {
	Name           string
	Description    string
	IconURL        string
	Image          string
	DefaultPorts   []database.PortMapping
	DefaultVolumes []database.VolumeMapping
	DefaultEnvVars map[string]string
}

// GetServiceTemplates returns all available service templates
func GetServiceTemplates() map[string]ServiceTemplate {
	return map[string]ServiceTemplate{
		"plex": {
			Name:        "Plex Media Server",
			Description: "Media server for streaming your content",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/plex.png",
			Image:       "lscr.io/linuxserver/plex:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 32400, Container: 32400, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/plex/config", Container: "/config"},
				{Host: "/mnt/media", Container: "/media"},
			},
			DefaultEnvVars: map[string]string{
				"PUID":    "1000",
				"PGID":    "1000",
				"TZ":      "America/New_York",
				"VERSION": "docker",
			},
		},
		"sonarr": {
			Name:        "Sonarr",
			Description: "TV series management and automation",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/sonarr.png",
			Image:       "lscr.io/linuxserver/sonarr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 8989, Container: 8989, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/sonarr/config", Container: "/config"},
				{Host: "/mnt/media/tv", Container: "/tv"},
				{Host: "/mnt/downloads", Container: "/downloads"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"radarr": {
			Name:        "Radarr",
			Description: "Movie collection management",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/radarr.png",
			Image:       "lscr.io/linuxserver/radarr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 7878, Container: 7878, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/radarr/config", Container: "/config"},
				{Host: "/mnt/media/movies", Container: "/movies"},
				{Host: "/mnt/downloads", Container: "/downloads"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"qbittorrent": {
			Name:        "qBittorrent",
			Description: "BitTorrent client with web interface",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/qbittorrent.png",
			Image:       "lscr.io/linuxserver/qbittorrent:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 8081, Container: 8080, Protocol: "tcp"},
				{Host: 6881, Container: 6881, Protocol: "tcp"},
				{Host: 6881, Container: 6881, Protocol: "udp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/qbittorrent/config", Container: "/config"},
				{Host: "/mnt/downloads", Container: "/downloads"},
			},
			DefaultEnvVars: map[string]string{
				"PUID":       "1000",
				"PGID":       "1000",
				"TZ":         "America/New_York",
				"WEBUI_PORT": "8080",
			},
		},
		"overseerr": {
			Name:        "Overseerr",
			Description: "Request management and media discovery",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/overseerr.png",
			Image:       "lscr.io/linuxserver/overseerr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 5055, Container: 5055, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/overseerr/config", Container: "/config"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"sabnzbd": {
			Name:        "SABnzbd",
			Description: "Usenet downloader",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/sabnzbd.png",
			Image:       "lscr.io/linuxserver/sabnzbd:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 8082, Container: 8080, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/sabnzbd/config", Container: "/config"},
				{Host: "/mnt/downloads", Container: "/downloads"},
				{Host: "/mnt/downloads/incomplete", Container: "/incomplete-downloads"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"prowlarr": {
			Name:        "Prowlarr",
			Description: "Indexer manager for Sonarr/Radarr",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/prowlarr.png",
			Image:       "lscr.io/linuxserver/prowlarr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 9696, Container: 9696, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/prowlarr/config", Container: "/config"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"bazarr": {
			Name:        "Bazarr",
			Description: "Subtitle management for Sonarr/Radarr",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/bazarr.png",
			Image:       "lscr.io/linuxserver/bazarr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 6767, Container: 6767, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/bazarr/config", Container: "/config"},
				{Host: "/mnt/media/movies", Container: "/movies"},
				{Host: "/mnt/media/tv", Container: "/tv"},
			},
			DefaultEnvVars: map[string]string{
				"PUID": "1000",
				"PGID": "1000",
				"TZ":   "America/New_York",
			},
		},
		"filebrowser": {
			Name:        "FileBrowser",
			Description: "Web-based file manager",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/filebrowser.png",
			Image:       "filebrowser/filebrowser:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 8083, Container: 80, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt", Container: "/srv"},
				{Host: "/mnt/docker/filebrowser/database", Container: "/database"},
				{Host: "/mnt/docker/filebrowser/config", Container: "/config"},
			},
			DefaultEnvVars: map[string]string{},
		},
		"homarr": {
			Name:        "Homarr",
			Description: "Customizable dashboard for your services",
			IconURL:     "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/homarr.png",
			Image:       "ghcr.io/ajnart/homarr:latest",
			DefaultPorts: []database.PortMapping{
				{Host: 7575, Container: 7575, Protocol: "tcp"},
			},
			DefaultVolumes: []database.VolumeMapping{
				{Host: "/mnt/docker/homarr/configs", Container: "/app/data/configs"},
				{Host: "/mnt/docker/homarr/icons", Container: "/app/public/icons"},
				{Host: "/mnt/docker/homarr/data", Container: "/data"},
			},
			DefaultEnvVars: map[string]string{},
		},
	}
}

// ShowContainerForm displays the container add/edit form
func (h *ContainerHandlers) ShowContainerForm(w http.ResponseWriter, r *http.Request) {
	templateName := r.URL.Query().Get("template")
	idStr := r.URL.Query().Get("id")

	// Get user's dark mode preference
	userID, _ := auth.GetUserSession(r)
	user, err := auth.GetUserByID(h.DB, userID)
	darkMode := false
	if err == nil {
		darkMode = user.DarkMode
	}

	data := map[string]interface{}{
		"IsEdit":    false,
		"Container": ContainerFormData{},
		"DarkMode":  darkMode,
	}

	// If editing existing container
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid container ID", http.StatusBadRequest)
			return
		}

		container, err := (&database.DB{DB: h.DB}).GetContainer(id)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get container")
			http.Error(w, "Container not found", http.StatusNotFound)
			return
		}

		data["IsEdit"] = true
		data["Container"] = ContainerFormData{
			ID:             container.ID,
			Name:           container.Name,
			Image:          container.Image,
			IconURL:        container.IconURL,
			PortMappings:   container.Ports,
			VolumeMappings: container.Volumes,
			EnvVars:        container.Environment,
		}
	} else if templateName != "" {
		// Load template for quick start
		templates := GetServiceTemplates()
		if tmpl, ok := templates[templateName]; ok {
			data["Container"] = ContainerFormData{
				Name:           templateName,
				Image:          tmpl.Image,
				IconURL:        tmpl.IconURL,
				PortMappings:   tmpl.DefaultPorts,
				VolumeMappings: tmpl.DefaultVolumes,
				EnvVars:        tmpl.DefaultEnvVars,
			}
			data["ServicePreview"] = ServicePreview{
				Name:        tmpl.Name,
				Description: tmpl.Description,
				IconURL:     tmpl.IconURL,
			}
		}
	}

	h.Templates.ExecuteTemplate(w, "container-form.html", data)
}

// SaveContainer handles creating/updating a container
func (h *ContainerHandlers) SaveContainer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Parse basic fields
	name := r.FormValue("name")
	image := r.FormValue("image")
	iconURL := r.FormValue("icon_url")
	idStr := r.FormValue("id")

	// Validate required fields
	if name == "" || image == "" {
		log.Error().Str("name", name).Str("image", image).Msg("Missing required fields")
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Check if this is a new container (id == 0) and if a container with this name already exists
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id == 0 {
		db := &database.DB{DB: h.DB}
		containers, err := db.ListContainers()
		if err == nil {
			for _, c := range containers {
				if c.Name == name {
					log.Warn().Str("name", name).Msg("Container name already exists")
					data := map[string]interface{}{
						"Error": fmt.Sprintf("A container named '%s' already exists. Please use a different name or delete the existing one first.", name),
						"Container": ContainerFormData{
							Name:  name,
							Image: image,
						},
					}
					h.Templates.ExecuteTemplate(w, "container-form.html", data)
					return
				}
			}
		}
	}

	// Parse port mappings
	portHosts := r.Form["port_host[]"]
	portContainers := r.Form["port_container[]"]
	portProtocols := r.Form["port_protocol[]"]

	var ports []database.PortMapping
	for i := range portHosts {
		if portHosts[i] == "" || portContainers[i] == "" {
			continue
		}
		host, _ := strconv.Atoi(portHosts[i])
		container, _ := strconv.Atoi(portContainers[i])
		protocol := "tcp"
		if i < len(portProtocols) && portProtocols[i] != "" {
			protocol = portProtocols[i]
		}
		ports = append(ports, database.PortMapping{
			Host:      host,
			Container: container,
			Protocol:  protocol,
		})
	}

	// Parse volume mappings
	volumeHosts := r.Form["volume_host[]"]
	volumeContainers := r.Form["volume_container[]"]

	var volumes []database.VolumeMapping
	for i := range volumeHosts {
		if volumeHosts[i] == "" || volumeContainers[i] == "" {
			continue
		}
		volumes = append(volumes, database.VolumeMapping{
			Host:      volumeHosts[i],
			Container: volumeContainers[i],
		})
	}

	// Parse environment variables
	envKeys := r.Form["env_key[]"]
	envValues := r.Form["env_value[]"]

	envVars := make(map[string]string)
	for i := range envKeys {
		if envKeys[i] == "" {
			continue
		}
		value := ""
		if i < len(envValues) {
			value = envValues[i]
		}
		envVars[envKeys[i]] = value
	}

	// Generate docker-compose file
	// Use ./data/compose for development, can be configured later
	composeDir := filepath.Join(".", "data", "compose")
	composePath := filepath.Join(composeDir, name+".yml")

	log.Info().Str("composeDir", composeDir).Msg("Creating compose directory")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		log.Error().Err(err).Msg("Failed to create compose directory")
		http.Error(w, "Failed to create container", http.StatusInternalServerError)
		return
	}

	composeContent := generateDockerCompose(name, image, ports, volumes, envVars)
	log.Info().Str("composePath", composePath).Msg("Writing compose file")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		log.Error().Err(err).Msg("Failed to write compose file")
		http.Error(w, "Failed to create container", http.StatusInternalServerError)
		return
	}

	log.Info().Msg("Creating database entry")
	db := &database.DB{DB: h.DB}

	// Check if this is an update (id > 0) or create (id == 0 or empty)
	// id was already parsed earlier for duplicate check
	if id > 0 {
		// Update existing container - first get the existing one to preserve DockerID
		log.Info().Int64("id", id).Msg("Updating existing container")
		existingContainer, err := db.GetContainer(id)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get existing container")
			http.Error(w, "Container not found", http.StatusNotFound)
			return
		}

		container := &database.Container{
			ID:          id,
			Name:        name,
			ServiceType: "",
			Image:       image,
			IconURL:     iconURL,
			Ports:       ports,
			Volumes:     volumes,
			Environment: envVars,
			ComposePath: composePath,
			Enabled:     true,
			DockerID:    existingContainer.DockerID, // Preserve existing DockerID
		}

		if err := db.UpdateContainer(container); err != nil {
			log.Error().Err(err).Msg("Failed to update container")
			http.Error(w, "Failed to update container", http.StatusInternalServerError)
			return
		}
	} else {
		// Create new container
		log.Info().Str("name", name).Msg("Creating new container in database")
		container := &database.Container{
			Name:        name,
			ServiceType: "",
			Image:       image,
			IconURL:     iconURL,
			Ports:       ports,
			Volumes:     volumes,
			Environment: envVars,
			ComposePath: composePath,
			Enabled:     true,
		}

		if err := db.CreateContainer(container); err != nil {
			log.Error().Err(err).Msg("Failed to create container in database")
			http.Error(w, "Failed to create container", http.StatusInternalServerError)
			return
		}
		log.Info().Msg("Container saved to database")

		// Create and start the Docker container
		ctx := context.Background()
		log.Info().Str("name", name).Str("image", image).Msg("Attempting to create and start container")
		dockerID, err := createAndStartContainer(ctx, name, image, ports, volumes, envVars)
		if err != nil {
			log.Error().Err(err).Str("name", name).Msg("Failed to create/start container")
			// Don't fail the request, container config is saved but not started
		} else {
			log.Info().Str("dockerID", dockerID).Str("name", name).Msg("Container created successfully")
			// Update the database with Docker container ID
			container.DockerID = dockerID
			if err := db.UpdateContainer(container); err != nil {
				log.Error().Err(err).Msg("Failed to update container with Docker ID")
			} else {
				log.Info().Str("name", name).Msg("Database updated with Docker ID")
			}
		}
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// generateDockerCompose generates a docker-compose YAML file
func generateDockerCompose(name, image string, ports []database.PortMapping, volumes []database.VolumeMapping, envVars map[string]string) string {
	compose := fmt.Sprintf("version: '3.8'\n\nservices:\n  %s:\n", name)
	compose += fmt.Sprintf("    container_name: %s\n", name)
	compose += fmt.Sprintf("    image: %s\n", image)

	// Add environment variables
	if len(envVars) > 0 {
		compose += "    environment:\n"
		for key, value := range envVars {
			compose += fmt.Sprintf("      - %s=%s\n", key, value)
		}
	}

	// Add volumes
	if len(volumes) > 0 {
		compose += "    volumes:\n"
		for _, v := range volumes {
			compose += fmt.Sprintf("      - %s:%s\n", v.Host, v.Container)
		}
	}

	// Add ports
	if len(ports) > 0 {
		compose += "    ports:\n"
		for _, p := range ports {
			if p.Protocol == "udp" {
				compose += fmt.Sprintf("      - \"%d:%d/udp\"\n", p.Host, p.Container)
			} else {
				compose += fmt.Sprintf("      - \"%d:%d\"\n", p.Host, p.Container)
			}
		}
	}

	compose += "    restart: unless-stopped\n"

	return compose
}

// startDockerCompose starts a docker-compose file
func startDockerCompose(composePath string) error {
	ctx := context.Background()

	// For now, we'll use the Docker SDK directly to create and start the container
	// In the future, we could shell out to docker-compose CLI
	log.Info().Str("path", composePath).Msg("Starting container from compose file")

	// We'll rely on the Docker SDK container creation in the calling function
	// The compose file is generated for reference and future use
	_ = ctx
	return nil
}

// createAndStartContainer creates and starts a Docker container
func createAndStartContainer(ctx context.Context, name, image string, ports []database.PortMapping, volumes []database.VolumeMapping, envVars map[string]string) (string, error) {
	client, err := docker.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer client.Close()

	// Pull the image first
	log.Info().Str("image", image).Msg("Pulling Docker image")
	if err := client.PullImage(ctx, image); err != nil {
		log.Warn().Err(err).Msg("Failed to pull image, will try to use local image")
	}

	// Convert ports to Docker format
	portBindings := make([]docker.ContainerPortBinding, 0, len(ports))
	for _, p := range ports {
		portBindings = append(portBindings, docker.ContainerPortBinding{
			ContainerPort: p.Container,
			HostPort:      p.Host,
			Protocol:      p.Protocol,
		})
	}

	// Convert volumes to Docker format
	volumeBindings := make([]string, 0, len(volumes))
	for _, v := range volumes {
		volumeBindings = append(volumeBindings, fmt.Sprintf("%s:%s", v.Host, v.Container))
	}

	// Convert env vars to Docker format
	envList := make([]string, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Create the container
	log.Info().Str("name", name).Msg("Creating Docker container")
	containerID, err := client.CreateContainer(ctx, name, image, portBindings, volumeBindings, envList)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	log.Info().Str("id", containerID).Msg("Starting Docker container")
	if err := client.StartContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return containerID, nil
}

// DeleteContainer handles deleting a container
func (h *ContainerHandlers) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	// Get container ID from URL
	idStr := r.URL.Path[len("/containers/delete/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid container ID", http.StatusBadRequest)
		return
	}

	db := &database.DB{DB: h.DB}

	// Get container from database
	container, err := db.GetContainer(id)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get container")
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	// Stop and remove Docker container if it exists
	if container.DockerID != "" {
		ctx := context.Background()
		client, err := docker.NewClient()
		if err == nil {
			defer client.Close()

			// Try to stop the container first
			log.Info().Str("id", container.DockerID).Msg("Stopping container")
			client.StopContainer(ctx, container.DockerID)

			// Remove the container
			log.Info().Str("id", container.DockerID).Msg("Removing container")
			if err := client.RemoveContainer(ctx, container.DockerID); err != nil {
				log.Warn().Err(err).Msg("Failed to remove Docker container")
			}
		}
	}

	// Delete from database
	if err := db.DeleteContainer(id); err != nil {
		log.Error().Err(err).Msg("Failed to delete container from database")
		http.Error(w, "Failed to delete container", http.StatusInternalServerError)
		return
	}

	// Redirect back to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
