package handler

import (
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// pathMethod represents an app path and method.
type pathMethod struct {
	path   string
	method string
}

// newMiddlewareRootPathSkipper returns a middleware skipper function that returns true
// if the request starts with the skipped path in the slice.
func newMiddlewareRootPathSkipper(sp []pathMethod) middleware.Skipper {
	return func(c echo.Context) bool {
		path := c.Request().URL.Path
		method := c.Request().Method

		for _, p := range sp {
			if strings.HasPrefix(path, p.path) && method == p.method {
				return true
			}
		}
		return false
	}
}
