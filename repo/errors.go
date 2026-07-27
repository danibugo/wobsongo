package repo

import "errors"

// Sentinel errors specific to repo implementations that don't map to a
// shared internal/data sentinel.
var (
	// ErrEmptyMediaKey indicates an S3 key was empty.
	ErrEmptyMediaKey = errors.New("media key cannot be empty")

	// ErrInvalidMalformedMediaKey indicates an S3 key failed format validation.
	ErrInvalidMalformedMediaKey = errors.New("invalid or malformed media key")
)
