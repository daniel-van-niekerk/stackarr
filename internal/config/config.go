package config

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string
	Port string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string // Path to SQLite database file
}

// New returns a Config with sensible defaults
func New() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: "8877",
		},
		Database: DatabaseConfig{
			Path: "./data/stackarr.db",
		},
	}
}
