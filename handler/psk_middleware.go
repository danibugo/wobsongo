// internal/handler/psk_middleware.go
package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/kairosedubf/wobsongo/model"
	"github.com/labstack/echo/v4"
)

// PSKAuthMiddleware returns an Echo middleware that authenticates incoming requests
// using a Pre-Shared Key (PSK).
//
// It expects the "Authorization" header to follow the format "PSK <key>".
// To prevent timing attacks, it securely compares the provided key against
// the expectedKey using a constant-time comparison.
//
// If the header is missing, lacks the correct prefix, or the provided key is invalid,
// it halts the request and returns a 401 Unauthorized response.
func PSKAuthMiddleware(expectedKey string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			const prefix = "PSK "

			// Check if the Authorization header contains the required "PSK " prefix
			if !strings.HasPrefix(header, prefix) {
				return &model.APIError{
					Code:     http.StatusUnauthorized,
					Internal: errors.New("missing PSK prefix in Authorization header"),
					Public:   "missing PSK",
				}
			}

			// Extract the key part from the header
			provided := strings.TrimPrefix(header, prefix)

			if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedKey)) != 1 {
				return &model.APIError{
					Code:     http.StatusUnauthorized,
					Internal: errors.New("PSK mismatch"),
					Public:   "invalid PSK",
				}
			}
			return next(c)
		}
	}
}
