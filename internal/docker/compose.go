package docker

import (
	"fmt"

	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/rs/zerolog/log"
)

// GenerateDockerCompose generates a docker-compose YAML file from container configuration
func GenerateDockerCompose(name, image string, ports []database.PortMapping, volumes []database.VolumeMapping, envVars map[string]string, networkMode string, privileged bool) string {
	compose := fmt.Sprintf("version: '3.8'\n\nservices:\n  %s:\n", name)
	compose += fmt.Sprintf("    container_name: %s\n", name)
	compose += fmt.Sprintf("    image: %s\n", image)

	// Add network mode if specified
	if networkMode == "host" {
		compose += "    network_mode: host\n"
	}

	// Add privileged mode if specified
	if privileged {
		compose += "    privileged: true\n"
	}

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

	// Add ports (skip when using host networking)
	if len(ports) > 0 && networkMode != "host" {
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

// StartDockerCompose starts a docker-compose file
// Currently unused - kept for future reference when docker-compose CLI integration is added
func StartDockerCompose(composePath string) error {
	// For now, we'll use the Docker SDK directly to create and start the container
	// In the future, we could shell out to docker-compose CLI
	log.Info().Str("path", composePath).Msg("Starting container from compose file")

	// We'll rely on the Docker SDK container creation in the calling function
	// The compose file is generated for reference and future use
	return nil
}
