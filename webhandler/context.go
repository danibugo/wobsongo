package webhandler

import (
	"github.com/kairosedubf/wobsongo/handler"
	"github.com/kairosedubf/wobsongo/view/layout"
	"github.com/labstack/echo/v4"
)

// csrfContextKey matches middleware.DefaultCSRFConfig.ContextKey — the key
// Echo's CSRF middleware stores the per-request token under.
const csrfContextKey = "csrf"

func buildAppLayout(c echo.Context, title, subtitle string) layout.AppLayoutData {
	claims := handler.ClaimsFromContext(c)
	d := layout.AppLayoutData{
		CurrentPath: c.Request().URL.Path,
		PageTitle:   title,
		Subtitle:    subtitle,
		CSRFToken:   csrfToken(c),
	}
	if claims != nil {
		d.User = layout.CurrentUser{ID: claims.UserID, Name: claims.Name, Role: claims.Role}
	}
	return d
}

// csrfToken returns the per-request CSRF token set by Echo's CSRF
// middleware, or "" if the middleware isn't in the chain (e.g. in tests).
func csrfToken(c echo.Context) string {
	if t, ok := c.Get(csrfContextKey).(string); ok {
		return t
	}
	return ""
}
