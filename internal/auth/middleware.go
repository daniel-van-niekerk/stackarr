package auth

import (
	"database/sql"
	"net/http"
)

var DB *sql.DB

// SetDB sets the database connection for middleware
func SetDB(db *sql.DB) {
	DB = db
}

// RequireAuth is middleware that requires authentication
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if user is authenticated
		userID, authenticated := GetUserSession(r)

		if !authenticated {
			// Redirect to login
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Verify user still exists in database
		if DB != nil {
			_, err := GetUserByID(DB, userID)
			if err != nil {
				// User doesn't exist (probably different database), clear session
				ClearSession(w, r)

				// Check if any users exist
				hasUsers, err := HasUsers(DB)
				if err == nil && !hasUsers {
					// No users, redirect to setup
					http.Redirect(w, r, "/setup", http.StatusSeeOther)
				} else {
					// Users exist, redirect to login
					http.Redirect(w, r, "/login", http.StatusSeeOther)
				}
				return
			}
		}

		// User is authenticated, continue to next handler
		next.ServeHTTP(w, r)
	})
}

// RedirectIfAuthenticated redirects to dashboard if already logged in
func RedirectIfAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if user is authenticated
		_, authenticated := GetUserSession(r)

		if authenticated {
			// Already logged in, redirect to dashboard
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}

		// Not authenticated, continue
		next.ServeHTTP(w, r)
	})
}
