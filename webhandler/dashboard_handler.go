package webhandler

import (
	dashboardview "github.com/kairosedubf/wobsongo/view/dashboard"
	"github.com/labstack/echo/v4"
)

// DashboardHandler handles the authenticated dashboard landing page.
type DashboardHandler struct{}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{}
}

func (h *DashboardHandler) dashboardPage(c echo.Context) error {
	layoutData := buildAppLayout(c, "Dashboard")
	return dashboardview.Index(layoutData).Render(c.Request().Context(), c.Response())
}
