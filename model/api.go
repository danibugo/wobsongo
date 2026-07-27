package model

import (
	"fmt"

	"github.com/google/uuid"
)

// APIResponse is the standard response shape returned by every handler.
type APIResponse struct {
	RequestID uuid.UUID `json:"request_id"               validate:"required"                 format:"uuid"`
	Status    int       `json:"status"                   validate:"required,gte=100,lte=599"`
	Data      any       `json:"data,omitempty,omitzero"`
	Error     string    `json:"error,omitempty,omitzero"`
}

// APIError is the standard error wrapper returned by handlers.
// Middlewares also expect to handle this error type.
type APIError struct {
	Code     int
	Internal error
	Public   string
}

// Error returns the original wrapped error.
// Satisfies the error interface.
func (e *APIError) Error() string {
	if e.Internal != nil {
		return e.Internal.Error()
	}
	// Provide a default message if Internal is nil
	return fmt.Sprintf("API Error: Code %d, Public: %s", e.Code, e.Public)
}
