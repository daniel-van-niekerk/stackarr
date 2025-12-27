package handlers

import (
	"html/template"
	"net/http"

	"github.com/daniel-van-niekerk/stackarr/internal/auth"
)

// DashboardHandlers holds dependencies for dashboard handlers
type DashboardHandlers struct {
	Templates *template.Template
}

// ShowDashboard displays the main dashboard
func (h *DashboardHandlers) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	// Get user ID from session
	userID, _ := auth.GetUserSession(r)

	// Render dashboard template
	h.Templates.ExecuteTemplate(w, "dashboard.html", map[string]interface{}{
		"UserID": userID,
	})
}
