package webhandler

import (
	"net/http"

	"github.com/kairosedubf/wobsongo/handler"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/labstack/echo/v4"
)

// RequireAuthMiddleware redirects unauthenticated requests to /login.
func RequireAuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if handler.ClaimsFromContext(c) == nil {
				return c.Redirect(http.StatusFound, "/login")
			}
			return next(c)
		}
	}
}

// RequireRoleMiddleware renders a 403 error page if the authenticated user's
// role is below minimum. Also redirects unauthenticated requests to /login.
// Not yet applied to any route — reserved for gating future admin-only pages.
func RequireRoleMiddleware(minimum model.UserRole) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := handler.ClaimsFromContext(c)
			if claims == nil {
				return c.Redirect(http.StatusFound, "/login")
			}
			if !claims.Role.AtLeast(minimum) {
				return renderHTMLError(c, http.StatusForbidden,
					"You don't have permission to access this page.")
			}
			return next(c)
		}
	}
}
