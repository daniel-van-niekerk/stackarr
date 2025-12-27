package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Container represents a managed container in the database
type Container struct {
	ID          int64
	Name        string
	ServiceType string
	Image       string
	Ports       []PortMapping
	Volumes     []VolumeMapping
	Environment map[string]string
	ComposePath string
	Enabled     bool
	DockerID    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PortMapping represents a port mapping
type PortMapping struct {
	Host      int    `json:"host"`
	Container int    `json:"container"`
	Protocol  string `json:"protocol"` // tcp or udp
}

// VolumeMapping represents a volume mapping
type VolumeMapping struct {
	Host      string `json:"host"`
	Container string `json:"container"`
}

// CreateContainer creates a new container record
func (db *DB) CreateContainer(c *Container) error {
	// Marshal JSON fields
	ports, err := json.Marshal(c.Ports)
	if err != nil {
		return fmt.Errorf("failed to marshal ports: %w", err)
	}

	volumes, err := json.Marshal(c.Volumes)
	if err != nil {
		return fmt.Errorf("failed to marshal volumes: %w", err)
	}

	environment, err := json.Marshal(c.Environment)
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	query := `
		INSERT INTO containers (name, service_type, image, ports, volumes, environment, compose_path, enabled, docker_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.Exec(query, c.Name, c.ServiceType, c.Image, string(ports), string(volumes), string(environment), c.ComposePath, c.Enabled, c.DockerID)
	if err != nil {
		return fmt.Errorf("failed to insert container: %w", err)
	}

	c.ID, _ = result.LastInsertId()
	return nil
}

// GetContainer retrieves a container by ID
func (db *DB) GetContainer(id int64) (*Container, error) {
	query := `SELECT id, name, service_type, image, ports, volumes, environment, compose_path, enabled, docker_id, created_at, updated_at FROM containers WHERE id = ?`

	c := &Container{}
	var ports, volumes, environment string

	err := db.QueryRow(query, id).Scan(&c.ID, &c.Name, &c.ServiceType, &c.Image, &ports, &volumes, &environment, &c.ComposePath, &c.Enabled, &c.DockerID, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("container not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query container: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(ports), &c.Ports); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ports: %w", err)
	}
	if err := json.Unmarshal([]byte(volumes), &c.Volumes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal volumes: %w", err)
	}
	if err := json.Unmarshal([]byte(environment), &c.Environment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal environment: %w", err)
	}

	return c, nil
}

// ListContainers retrieves all containers
func (db *DB) ListContainers() ([]*Container, error) {
	query := `SELECT id, name, service_type, image, ports, volumes, environment, compose_path, enabled, docker_id, created_at, updated_at FROM containers ORDER BY created_at DESC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query containers: %w", err)
	}
	defer rows.Close()

	containers := make([]*Container, 0)
	for rows.Next() {
		c := &Container{}
		var ports, volumes, environment string

		err := rows.Scan(&c.ID, &c.Name, &c.ServiceType, &c.Image, &ports, &volumes, &environment, &c.ComposePath, &c.Enabled, &c.DockerID, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan container: %w", err)
		}

		// Unmarshal JSON fields
		json.Unmarshal([]byte(ports), &c.Ports)
		json.Unmarshal([]byte(volumes), &c.Volumes)
		json.Unmarshal([]byte(environment), &c.Environment)

		containers = append(containers, c)
	}

	return containers, nil
}

// UpdateContainer updates a container record
func (db *DB) UpdateContainer(c *Container) error {
	ports, _ := json.Marshal(c.Ports)
	volumes, _ := json.Marshal(c.Volumes)
	environment, _ := json.Marshal(c.Environment)

	query := `
		UPDATE containers
		SET name = ?, service_type = ?, image = ?, ports = ?, volumes = ?, environment = ?, compose_path = ?, enabled = ?, docker_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := db.Exec(query, c.Name, c.ServiceType, c.Image, string(ports), string(volumes), string(environment), c.ComposePath, c.Enabled, c.DockerID, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update container: %w", err)
	}

	return nil
}

// DeleteContainer deletes a container by ID
func (db *DB) DeleteContainer(id int64) error {
	query := `DELETE FROM containers WHERE id = ?`
	_, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}
	return nil
}
