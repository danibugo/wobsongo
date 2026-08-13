package model

import (
	"errors"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	t.Run("with internal error", func(t *testing.T) {
		e := &APIError{Code: 500, Internal: errors.New("db connection failed"), Public: "internal error"}
		if got, want := e.Error(), "db connection failed"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without internal error", func(t *testing.T) {
		e := &APIError{Code: 404, Public: "not found"}
		want := "API Error: Code 404, Public: not found"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}
