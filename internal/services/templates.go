package services

import (
	"strings"

	"github.com/daniel-van-niekerk/stackarr/internal/database"
)

// Template represents a pre-configured service template
type Template struct {
	Name           string
	Description    string
	IconURL        string
	Image          string
	DefaultPorts   []database.PortMapping
	DefaultVolumes []database.VolumeMapping
	DefaultEnvVars map[string]string
}

// Registry manages service templates
type Registry struct {
	templates map[string]Template
}

// NewRegistry creates a new template registry with all available templates
func NewRegistry() *Registry {
	return &Registry{
		templates: map[string]Template{
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
		},
	}
}

// Get returns a template by name
func (r *Registry) Get(name string) (Template, bool) {
	t, ok := r.templates[name]
	return t, ok
}

// List returns all available template names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}

// DetectInstalled checks which templates are installed based on container images
func (r *Registry) DetectInstalled(containerImages []string) map[string]bool {
	installed := make(map[string]bool)

	for _, image := range containerImages {
		imageLower := strings.ToLower(image)
		for name := range r.templates {
			if strings.Contains(imageLower, name) {
				installed[name] = true
				break // Once we find a match, move to next image
			}
		}
	}

	return installed
}
