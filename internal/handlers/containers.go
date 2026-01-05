package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/daniel-van-niekerk/stackarr/internal/services"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/rs/zerolog/log"
)

// ContainerHandlers holds dependencies for container handlers
type ContainerHandlers struct {
	DB              *sql.DB
	Templates       *template.Template
	ProgressManager *streaming.ProgressManager
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
		registry := services.NewRegistry()
		if tmpl, ok := registry.Get(templateName); ok {
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
	// Read the body (required for multipart parsing)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to read request body",
		})
		return
	}

	// Recreate the body so parsing can read it
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// For multipart/form-data, use ParseMultipartForm with a larger max memory allocation
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && bytes.Contains([]byte(contentType), []byte("multipart/form-data")) {
		// Use ParseMultipartForm for multipart data
		// 32MB max size for the entire multipart message
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			log.Error().
				Err(err).
				Str("content_type", contentType).
				Msg("Failed to parse multipart form")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid form data",
			})
			return
		}
	} else {
		// Fall back to regular form parsing for application/x-www-form-urlencoded
		if err := r.ParseForm(); err != nil {
			log.Error().Err(err).Msg("Failed to parse form")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid form data",
			})
			return
		}
	}

	// Parse basic fields (FormValue works on both r.Form and r.MultipartForm.Value)
	name := r.FormValue("name")
	image := r.FormValue("image")
	iconURL := r.FormValue("icon_url")
	idStr := r.FormValue("id")

	// Validate required fields
	if name == "" || image == "" {
		log.Error().Str("name", name).Str("image", image).Msg("Missing required fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Container name and image are required",
		})
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
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("A container named '%s' already exists. Please use a different name or delete the existing one first.", name),
					})
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

	// Generate random encryption key for Homarr if not provided
	if name == "homarr" && (envVars["SECRET_ENCRYPTION_KEY"] == "" || envVars["SECRET_ENCRYPTION_KEY"] == "0") {
		encryptionKey, err := generateRandomHex(32)
		if err != nil {
			log.Error().Err(err).Msg("Failed to generate encryption key for Homarr")
			http.Error(w, "Failed to generate encryption key", http.StatusInternalServerError)
			return
		}
		envVars["SECRET_ENCRYPTION_KEY"] = encryptionKey
		log.Info().Msg("Generated random encryption key for Homarr")
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

	composeContent := docker.GenerateDockerCompose(name, image, ports, volumes, envVars)
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

		// Return success JSON for edit case (no async operation needed for config updates)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "success",
			"redirect": "/dashboard",
		})
		return
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

		// Generate operation ID and launch async container creation
		operationID := fmt.Sprintf("create-%s-%d", name, time.Now().Unix())
		h.ProgressManager.CreateOperation(operationID)

		// Prepare data for async execution
		containerData := ContainerFormData{
			ID:             container.ID,
			Name:           name,
			Image:          image,
			IconURL:        iconURL,
			PortMappings:   ports,
			VolumeMappings: volumes,
			EnvVars:        envVars,
		}

		// Launch async operation
		go h.executeContainerCreate(operationID, containerData)

		// Return operation ID as JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"operation_id": operationID,
			"status":       "started",
		})
		return
	}
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

// generateRandomHex generates a random hex string of the specified length (in bytes)
// For a 64-character hex string, pass 32
func generateRandomHex(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
