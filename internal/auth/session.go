package auth

import (
	"net/http"

	"github.com/gorilla/sessions"
)

const (
	SessionName   = "stackarr-session"
	SessionUserID = "user_id"
)

// SessionStore holds the session store
var SessionStore *sessions.CookieStore

// InitSessionStore initializes the session store
func InitSessionStore(secret string) {
	SessionStore = sessions.NewCookieStore([]byte(secret))

	// Configure session options
	SessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400, // 24 hours in seconds
		HttpOnly: true,  // Prevents JavaScript access (XSS protection)
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
	}
}

// SetUserSession stores the user ID in the session
func SetUserSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	session, err := SessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values[SessionUserID] = userID
	return session.Save(r, w)
}

// GetUserSession retrieves the user ID from the session
func GetUserSession(r *http.Request) (int64, bool) {
	session, err := SessionStore.Get(r, SessionName)
	if err != nil {
		return 0, false
	}

	userID, ok := session.Values[SessionUserID].(int64)
	return userID, ok
}

// ClearSession removes the user session
func ClearSession(w http.ResponseWriter, r *http.Request) error {
	session, err := SessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values[SessionUserID] = nil
	session.Options.MaxAge = -1 // Delete the cookie
	return session.Save(r, w)
}
