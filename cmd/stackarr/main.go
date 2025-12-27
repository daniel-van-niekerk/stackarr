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
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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

	// Load HTML templates
	tmpl := template.Must(template.ParseFiles(
		"web/templates/login.html",
		"web/templates/setup.html",
		"web/templates/dashboard.html",
		"web/templates/docker-install.html",
	))
	log.Info().Msg("Templates loaded")

	// Initialize handlers
	authHandlers := &handlers.AuthHandlers{
		DB:        db.DB,
		Templates: tmpl,
	}
	dashboardHandlers := &handlers.DashboardHandlers{
		DB:        db.DB,
		Templates: tmpl,
	}

	// Create Chi router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

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
		r.Get("/docker-install", dashboardHandlers.ShowDockerInstall)

		// Container control endpoints
		r.Post("/containers/start", dashboardHandlers.StartContainer)
		r.Post("/containers/stop", dashboardHandlers.StopContainer)
		r.Post("/containers/restart", dashboardHandlers.RestartContainer)

		// User preferences
		r.Post("/preferences/toggle-external", dashboardHandlers.ToggleExternalContainers)

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
