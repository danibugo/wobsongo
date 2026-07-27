package webhandler

import (
	"net/http"
	"time"

	"github.com/kairosedubf/wobsongo/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// loginRateLimit returns a rate-limiting middleware for the login endpoint:
// 5 requests/minute per IP, burst of 2. No-op in testing mode.
func loginRateLimit(cfg *config.Config) echo.MiddlewareFunc {
	if cfg.IsTesting() {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error { return next(c) }
		}
	}
	rps := float64(5) / time.Minute.Seconds()
	store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      rate.Limit(rps),
		Burst:     2,
		ExpiresIn: time.Minute,
	})
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		IdentifierExtractor: func(c echo.Context) (string, error) {
			ip := c.RealIP()
			if ip == "" {
				ip = "unknown"
			}
			return ip, nil
		},
		Store: store,
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			return renderHTMLError(
				c,
				http.StatusTooManyRequests,
				"Too many attempts. Please wait a moment and try again.",
			)
		},
	})
}
