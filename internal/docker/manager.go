package docker

import (
	"context"
	"fmt"
)

// Manager handles Docker client lifecycle and operations
type Manager struct{}

// NewManager creates a new Docker manager
func NewManager() *Manager {
	return &Manager{}
}

// WithClient creates a Docker client, calls the provided function, and ensures cleanup.
// This pattern eliminates repeated client creation and cleanup boilerplate.
func (m *Manager) WithClient(ctx context.Context, fn func(*Client) error) error {
	client, err := NewClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer client.Close()

	return fn(client)
}
