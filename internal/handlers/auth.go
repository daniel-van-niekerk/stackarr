package handlers

import (
	"database/sql"
	"html/template"
	"net/http"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
	"github.com/rs/zerolog/log"
)

// AuthHandlers holds dependencies for auth handlers
type AuthHandlers struct {
	DB        *sql.DB
	Templates *template.Template
}

// ShowSetup displays the setup page (first-time admin creation)
func (h *AuthHandlers) ShowSetup(w http.ResponseWriter, r *http.Request) {
	// Check if users already exist
	hasUsers, err := auth.HasUsers(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check for users")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If users exist, redirect to login
	if hasUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Render setup template
	h.Templates.ExecuteTemplate(w, "setup.html", nil)
}

// HandleSetup processes the setup form
func (h *AuthHandlers) HandleSetup(w http.ResponseWriter, r *http.Request) {
	// Check if users already exist
	hasUsers, err := auth.HasUsers(h.DB)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check for users")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if hasUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Parse form
	username := r.FormValue("username")
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	// Validate
	if username == "" || password == "" {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]interface{}{
			"Error": "Username and password are required",
		})
		return
	}

	if len(password) < 8 {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]interface{}{
			"Error": "Password must be at least 8 characters",
		})
		return
	}

	if password != passwordConfirm {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]interface{}{
			"Error": "Passwords do not match",
		})
		return
	}

	// Create user
	err = auth.CreateUser(h.DB, username, password)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create user")
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]interface{}{
			"Error": "Failed to create user",
		})
		return
	}

	log.Info().Str("username", username).Msg("Admin user created")

	// Redirect to login
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ShowLogin displays the login page
func (h *AuthHandlers) ShowLogin(w http.ResponseWriter, r *http.Request) {
	h.Templates.ExecuteTemplate(w, "login.html", nil)
}

// HandleLogin processes the login form
func (h *AuthHandlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Parse form
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Authenticate
	user, err := auth.Authenticate(h.DB, username, password)
	if err != nil {
		log.Warn().Str("username", username).Msg("Failed login attempt")
		h.Templates.ExecuteTemplate(w, "login.html", map[string]interface{}{
			"Error": "Invalid username or password",
		})
		return
	}

	// Set session
	if err := auth.SetUserSession(w, r, user.ID); err != nil {
		log.Error().Err(err).Msg("Failed to set session")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Info().Str("username", username).Int64("user_id", user.ID).Msg("User logged in")

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// HandleLogout logs out the user
func (h *AuthHandlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Get user ID for logging
	userID, _ := auth.GetUserSession(r)

	// Clear session
	if err := auth.ClearSession(w, r); err != nil {
		log.Error().Err(err).Msg("Failed to clear session")
	}

	log.Info().Int64("user_id", userID).Msg("User logged out")

	// Redirect to login
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
