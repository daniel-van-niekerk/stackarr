package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Import SQLite driver
)

// DB wraps the sql.DB connection
type DB struct {
	*sql.DB
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	// Create the directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open the database connection
	// The DSN (Data Source Name) includes important SQLite pragmas
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite works best with a single connection
	db.SetMaxIdleConns(1)

	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// InitSchema creates the initial database schema
func (db *DB) InitSchema() error {
	// Create users table
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		show_external_containers BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.Exec(usersTable); err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Create containers table
	containersTable := `
	CREATE TABLE IF NOT EXISTS containers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		service_type TEXT NOT NULL,
		image TEXT NOT NULL,
		ports TEXT,
		volumes TEXT,
		environment TEXT,
		compose_path TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		docker_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_containers_service_type ON containers(service_type);
	`

	if _, err := db.Exec(containersTable); err != nil {
		return fmt.Errorf("failed to create containers table: %w", err)
	}

	// Run migrations
	if err := db.runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations applies database migrations
func (db *DB) runMigrations() error {
	// Create migrations table if it doesn't exist
	migrationsTable := `
	CREATE TABLE IF NOT EXISTS migrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(migrationsTable); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Define migrations
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "add_show_external_containers_to_users",
			sql: `
				ALTER TABLE users ADD COLUMN show_external_containers BOOLEAN DEFAULT 1;
			`,
		},
	}

	// Apply each migration
	for _, migration := range migrations {
		// Check if migration already applied
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM migrations WHERE name = ?", migration.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", migration.name, err)
		}

		if count > 0 {
			// Migration already applied, skip
			continue
		}

		// Apply migration
		if _, err := db.Exec(migration.sql); err != nil {
			// If column already exists, mark as applied anyway
			if err.Error() == "SQL logic error: duplicate column name: show_external_containers (1)" {
				// Mark as applied
				if _, err := db.Exec("INSERT INTO migrations (name) VALUES (?)", migration.name); err != nil {
					return fmt.Errorf("failed to record migration %s: %w", migration.name, err)
				}
				continue
			}
			return fmt.Errorf("failed to apply migration %s: %w", migration.name, err)
		}

		// Record migration as applied
		if _, err := db.Exec("INSERT INTO migrations (name) VALUES (?)", migration.name); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", migration.name, err)
		}
	}

	return nil
}
