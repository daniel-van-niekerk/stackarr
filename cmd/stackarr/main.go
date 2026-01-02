package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/daniel-van-niekerk/stackarr/internal/config"
	"github.com/daniel-van-niekerk/stackarr/internal/database"
	"github.com/daniel-van-niekerk/stackarr/internal/handlers"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Version is set at build time via -ldflags
var Version = "dev"

func main() {
	// Configure zerolog for human-readable console output
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	})

	// Set global log level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	log.Info().Msg("Starting StackArr...")

	// Load configuration
	cfg := config.New()

	// Initialize session store
	auth.InitSessionStore("your-secret-key-change-this-in-production")
	log.Info().Msg("Session store initialized")

	// Initialize database
	log.Info().Str("path", cfg.Database.Path).Msg("Opening database")
	db, err := database.New(cfg.Database.Path)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database")
	}
	defer db.Close()

	// Initialize database schema
	log.Info().Msg("Initializing database schema")
	if err := db.InitSchema(); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize schema")
	}
	log.Info().Msg("Database ready")

	// Check for user reset request
	resetToken := os.Getenv("RESET_USER")
	if resetToken != "" {
		if resetToken == "confirm-delete-user" {
			count, err := auth.DeleteAllUsers(db.DB)
			if err != nil {
				log.Error().
					Err(err).
					Str("action", "user_reset_failed").
					Msg("Failed to delete users during reset")
			} else if count == 0 {
				log.Info().
					Str("action", "user_reset").
					Msg("RESET_USER set but no users found")
			} else {
				log.Warn().
					Str("action", "user_reset").
					Int64("users_deleted", count).
					Msg("All users deleted via RESET_USER - setup required")
			}
		} else {
			log.Warn().
				Str("action", "user_reset_failed").
				Str("provided_token", resetToken).
				Msg("Invalid RESET_USER token - expected 'confirm-delete-user'")
		}
	}

	// Set database for auth middleware
	auth.SetDB(db.DB)

	// Initialize progress manager for SSE streaming
	progressManager := streaming.NewProgressManager()
	log.Info().Msg("Progress manager initialized")

	// Start cleanup goroutine for old operations
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			progressManager.CleanupOld(30 * time.Minute)
		}
	}()

	// Set application version for handlers
	handlers.AppVersion = Version
	log.Info().Str("version", Version).Msg("Application version")

	// Load HTML templates
	tmpl := template.Must(template.ParseFiles(
		"web/templates/login.html",
		"web/templates/setup.html",
		"web/templates/dashboard.html",
		"web/templates/container-form.html",
	))
	log.Info().Msg("Templates loaded")

	// Initialize handlers
	authHandlers := &handlers.AuthHandlers{
		DB:        db.DB,
		Templates: tmpl,
	}
	dashboardHandlers := &handlers.DashboardHandlers{
		DB:              db.DB,
		Templates:       tmpl,
		ProgressManager: progressManager,
	}
	containerHandlers := &handlers.ContainerHandlers{
		DB:              db.DB,
		Templates:       tmpl,
		ProgressManager: progressManager,
	}
	streamingHandlers := &handlers.StreamingHandlers{
		ProgressManager: progressManager,
	}

	// Create Chi router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Serve static files
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Public routes (no authentication required)
	r.Group(func(r chi.Router) {
		r.Use(auth.RedirectIfAuthenticated) // Redirect to dashboard if logged in

		r.Get("/setup", authHandlers.ShowSetup)
		r.Post("/setup", authHandlers.HandleSetup)
		r.Get("/login", authHandlers.ShowLogin)
		r.Post("/login", authHandlers.HandleLogin)
	})

	// Protected routes (authentication required)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth) // Require authentication

		r.Get("/dashboard", dashboardHandlers.ShowDashboard)

		// Container management
		r.Get("/containers/add", containerHandlers.ShowContainerForm)
		r.Post("/containers/save", containerHandlers.SaveContainer)
		r.Post("/containers/delete/{id}", containerHandlers.DeleteContainer)

		// Container control endpoints
		r.Post("/containers/start", dashboardHandlers.StartContainer)
		r.Post("/containers/stop", dashboardHandlers.StopContainer)
		r.Post("/containers/restart", dashboardHandlers.RestartContainer)
		r.Post("/containers/update", dashboardHandlers.UpdateContainer)

		// User preferences
		r.Post("/preferences/toggle-external", dashboardHandlers.ToggleExternalContainers)
		r.Post("/preferences/toggle-dark-mode", dashboardHandlers.ToggleDarkMode)

		// SSE progress streaming endpoint
		r.Get("/containers/progress/{operationID}", streamingHandlers.StreamProgress)

		r.Post("/logout", authHandlers.HandleLogout)
	})

	// Root route - redirect based on auth status
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, authenticated := auth.GetUserSession(r)
		if authenticated {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		} else {
			// Check if setup is needed
			hasUsers, err := auth.HasUsers(db.DB)
			if err != nil {
				log.Error().Err(err).Msg("Failed to check for users")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if !hasUsers {
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
		}
	})

	// Start HTTP server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Info().
		Str("addr", addr).
		Str("url", fmt.Sprintf("http://localhost:%s", cfg.Server.Port)).
		Msg("Starting HTTP server")

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}
}
