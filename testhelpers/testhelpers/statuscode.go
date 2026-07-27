// Package testhelpers centralizes test-support conventions shared across the
// project's test suites.
package testhelpers

import (
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
)

// Suffix* constants define the project-wide convention for simulating
// repo-layer outcomes from a fixture UUID: the trailing two hex characters
// of the UUID select which sentinel error (if any) a mock repo should
// return for that ID. Mocks across the project should resolve simulated
// errors through ErrorForUUID so every handler test suite gets deterministic
// status-code coverage without per-case closures.
const (
	// SuffixOK simulates the happy path — no error.
	SuffixOK = "00"

	// SuffixForbidden simulates data.ErrForbidden (HTTP 403).
	SuffixForbidden = "03"

	// SuffixNotFound simulates data.ErrNotFound (HTTP 404).
	SuffixNotFound = "04"

	// SuffixInternal simulates data.ErrInternal (HTTP 500).
	SuffixInternal = "05"

	// SuffixConflict simulates data.ErrConflict (HTTP 409).
	SuffixConflict = "09"
)

// suffixErrors maps a UUID's trailing two hex characters to the sentinel
// error a mock repo should simulate. A suffix with no entry (including
// SuffixOK) resolves to no error.
var suffixErrors = map[string]error{
	SuffixForbidden: data.ErrForbidden,
	SuffixNotFound:  data.ErrNotFound,
	SuffixInternal:  data.ErrInternal,
	SuffixConflict:  data.ErrConflict,
}

// ErrorForUUID returns the sentinel error a mock repo should simulate for id,
// based on its trailing two hex characters, or nil for the happy path.
func ErrorForUUID(id uuid.UUID) error {
	s := id.String()
	suffix := s[len(s)-2:]
	return suffixErrors[suffix]
}

// NewUUIDWithSuffix returns a random UUID whose trailing two hex characters
// are suffix, for building deterministic test fixtures, e.g.:
//
//	testhelpers.NewUUIDWithSuffix(testhelpers.SuffixNotFound)
func NewUUIDWithSuffix(suffix string) uuid.UUID {
	s := uuid.New().String()
	return uuid.MustParse(s[:len(s)-2] + suffix)
}
