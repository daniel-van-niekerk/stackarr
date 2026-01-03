package docker

import (
	"fmt"
	"os"

	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/rs/zerolog/log"
)

// CreateVolumeDirectories creates host directories for volume mounts
func CreateVolumeDirectories(volumes []database.VolumeMapping) error {
	for _, vol := range volumes {
		hostPath := vol.Host

		// Skip if it's a named volume (doesn't start with / or .)
		if len(hostPath) == 0 || (hostPath[0] != '/' && hostPath[0] != '.') {
			log.Debug().Str("path", hostPath).Msg("Skipping named volume")
			continue
		}

		// Check if path already exists
		exists := false
		if _, err := os.Stat(hostPath); err == nil {
			// Path exists
			exists = true
			log.Debug().Str("path", hostPath).Msg("Volume directory already exists")
		} else if !os.IsNotExist(err) {
			// Some other error occurred
			log.Warn().Err(err).Str("path", hostPath).Msg("Error checking volume path")
			continue
		}

		// Create directory if it doesn't exist
		if !exists {
			// Create directory with permissions 0777 (world-writable)
			// This ensures containers running as any user can write to it
			log.Info().Str("path", hostPath).Msg("Creating volume directory")
			if err := os.MkdirAll(hostPath, 0777); err != nil {
				log.Error().Err(err).Str("path", hostPath).Msg("Failed to create volume directory")
				return fmt.Errorf("failed to create directory %s: %w", hostPath, err)
			}
		}

		// Always try to set ownership to 1000:1000 (common for container users)
		// This may fail on Windows or if running without proper permissions
		log.Debug().Str("path", hostPath).Msg("Setting directory ownership to 1000:1000")
		if err := os.Chown(hostPath, 1000, 1000); err != nil {
			log.Debug().Err(err).Str("path", hostPath).Msg("Could not set directory ownership (this is normal on Windows)")
		}

		// Always ensure the directory is writable by changing permissions to 0777
		// This helps on systems where umask might restrict permissions
		log.Debug().Str("path", hostPath).Msg("Setting directory permissions to 0777")
		if err := os.Chmod(hostPath, 0777); err != nil {
			log.Warn().Err(err).Str("path", hostPath).Msg("Could not set directory permissions")
		}
	}

	return nil
}
