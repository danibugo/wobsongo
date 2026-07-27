package testhelpers

import "github.com/google/uuid"

// APIResponse is a generic, typed mirror of model.APIResponse for decoding
// JSON response bodies in tests without fighting the `any`-typed Data field.
type APIResponse[T any] struct {
	RequestID uuid.UUID `json:"request_id,omitempty"`
	Status    int       `json:"status"`
	Data      T         `json:"data,omitempty"`
	Error     string    `json:"error,omitempty"`
}
