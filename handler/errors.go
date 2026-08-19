package handler

import (
	"errors"
	"net/http"

	"github.com/kairosedubf/wobsongo/data"
)

// msgInvalidRequestBody is the public message returned when a handler fails
// to bind/validate a request body.
const msgInvalidRequestBody = "invalid request body"

// errorMapping defines the HTTP status code and public message for a sentinel error.
type errorMapping struct {
	statusCode int
	message    string
}

// dataErrorMappings maps internal/data sentinel errors to HTTP responses.
var dataErrorMappings = map[error]errorMapping{
	data.ErrNotFound: {
		statusCode: http.StatusNotFound,
		message:    "The requested resource could not be found",
	},
	data.ErrConflict: {
		statusCode: http.StatusConflict,
		message:    "Conflict with existing resource",
	},
	data.ErrForbidden: {
		statusCode: http.StatusForbidden,
		message:    "Forbidden: insufficient permissions",
	},
	data.ErrInternal: {
		statusCode: http.StatusInternalServerError,
		message:    "An unexpected internal error occurred",
	},
}

// mapDataErrorToHTTP maps a sentinel error from internal/data to an HTTP
// status code and public message, matching wrapped errors via errors.Is.
// Unmapped errors fall back to a generic 500.
func mapDataErrorToHTTP(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}

	for sentinel, mapping := range dataErrorMappings {
		if errors.Is(err, sentinel) {
			return mapping.statusCode, mapping.message
		}
	}

	return http.StatusInternalServerError, "An unexpected internal server error occurred"
}
