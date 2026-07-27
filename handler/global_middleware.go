package handler

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/model"
	errview "github.com/kairosedubf/wobsongo/view/error"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// requestIDMiddleware sets a unique request ID for each incoming request.
// Also sends the request ID in the response header as "X-Request-ID".
func requestIDMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		uid, err := uuid.NewV7()
		if err != nil {
			uid = uuid.New()
		}
		id := uid.String()
		c.Set(contextRequestID, id)
		c.Response().Header().Set(headerRequestID, id)
		return next(c)
	}
}

// corsMiddleware sets up CORS middleware to allow requests from configured origins.
func corsMiddleware() echo.MiddlewareFunc {
	config := config.NewConfig()
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:  config.CORSAllowedOrigins,
		AllowMethods:  config.CORSAllowedMethods,
		ExposeHeaders: []string{headerRequestID},
	})
}

// loggerMiddleware creates a logging middleware that logs details of each request.
func loggerMiddleware() echo.MiddlewareFunc {
	config := config.NewConfig()
	skipped := []pathMethod{
		{
			path:   "/docs",
			method: "GET",
		},
		{
			path:   "/static",
			method: "GET",
		},
	}
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper:     newMiddlewareRootPathSkipper(skipped),
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogRemoteIP: true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			err_ := ""
			if v.Error != nil {
				err_ = v.Error.Error()
			}
			reqID, ok := c.Get(contextRequestID).(string)
			if !ok {
				reqID = ""
			}
			// This check is required so that the app doesn't log nil errors as "error: <nil>"
			if err_ != "" {
				config.Logger.LogAttrs(c.Request().Context(), slog.LevelInfo, "REQUEST",
					slog.String(contextRequestID, reqID),
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("ip", v.RemoteIP),
					slog.String("latency", v.Latency.String()),
					slog.String("error", err_),
				)
			} else {
				config.Logger.LogAttrs(c.Request().Context(), slog.LevelInfo, "REQUEST",
					slog.String(contextRequestID, reqID),
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("ip", v.RemoteIP),
					slog.String("latency", v.Latency.String()),
				)
			}
			return nil
		},
	})
}

// customErrorHandler defines a custom error handler for the Echo instance.
// HTML (non-/api) routes get a rendered error page; /api routes keep the
// existing JSON error envelope.
func customErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		// If the response has already been sent, return early
		if c.Response().Committed {
			return
		}

		code := echo.ErrInternalServerError.Code
		var message string
		switch e := err.(type) {
		case *model.APIError:
			code = e.Code
			message = e.Public
		case *echo.HTTPError:
			code = e.Code
			message = fmt.Sprintf("%v", e.Message)
		default:
			message = e.Error()
		}

		if !strings.HasPrefix(c.Request().URL.Path, "/api") {
			c.Response().WriteHeader(code)
			_ = errview.Error(code, message).Render(c.Request().Context(), c.Response())
			return
		}

		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		reqID, ok := c.Get(contextRequestID).(string)
		if !ok {
			reqID = uuid.New().String()
		}

		requestID, _ := uuid.Parse(reqID)

		_ = c.JSON(code, model.APIResponse{
			Status:    code,
			Error:     message,
			RequestID: requestID,
		})
	}
}
