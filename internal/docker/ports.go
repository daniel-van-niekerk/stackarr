package docker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// PortConflict represents a port that's already in use
type PortConflict struct {
	Port          int
	ContainerID   string
	ContainerName string
}

// CheckPortConflicts checks if any of the given ports are already in use by Docker containers
func (c *Client) CheckPortConflicts(ctx context.Context, ports []int) ([]PortConflict, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get all running containers
	containers, err := c.ListContainers(ctx, false) // false = only running
	if err != nil {
		return nil, err
	}

	// Build a map of requested ports for quick lookup
	requestedPorts := make(map[int]bool)
	for _, port := range ports {
		requestedPorts[port] = true
	}

	// Check for conflicts
	conflicts := make([]PortConflict, 0)
	for _, container := range containers {
		for _, portBinding := range container.Ports {
			hostPort, err := strconv.Atoi(portBinding.HostPort)
			if err != nil {
				log.Warn().Str("port", portBinding.HostPort).Msg("Invalid port number")
				continue
			}

			// Check if this port is in our requested list
			if requestedPorts[hostPort] {
				conflicts = append(conflicts, PortConflict{
					Port:          hostPort,
					ContainerID:   container.ID,
					ContainerName: container.Name,
				})
			}
		}
	}

	return conflicts, nil
}

// GetUsedPorts returns all ports currently in use by Docker containers
func (c *Client) GetUsedPorts(ctx context.Context) ([]int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	containers, err := c.ListContainers(ctx, false) // Only running containers
	if err != nil {
		return nil, err
	}

	usedPorts := make(map[int]bool)
	for _, container := range containers {
		for _, portBinding := range container.Ports {
			if hostPort, err := strconv.Atoi(portBinding.HostPort); err == nil {
				usedPorts[hostPort] = true
			}
		}
	}

	// Convert map to slice
	ports := make([]int, 0, len(usedPorts))
	for port := range usedPorts {
		ports = append(ports, port)
	}

	return ports, nil
}

// FindAvailablePort finds the next available port starting from the given port
func (c *Client) FindAvailablePort(ctx context.Context, startPort int) (int, error) {
	usedPorts, err := c.GetUsedPorts(ctx)
	if err != nil {
		return 0, err
	}

	// Build map for O(1) lookup
	usedMap := make(map[int]bool)
	for _, port := range usedPorts {
		usedMap[port] = true
	}

	// Find next available port
	for port := startPort; port <= 65535; port++ {
		if !usedMap[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports found starting from %d", startPort)
}
