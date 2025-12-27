package auth

import (
	"net/http"
)

// RequireAuth is middleware that requires authentication
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if user is authenticated
		_, authenticated := GetUserSession(r)

		if !authenticated {
			// Redirect to login
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
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
