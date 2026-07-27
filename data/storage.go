// Package data provides interfaces and types for media storage operations,
// including presigned URL generation and S3 object management.
package data

import (
	"context"
	"io"
	"net/url"
	"slices"
	"time"
)

// MediaUploadIntent represents the intent of a media/file upload.
type MediaUploadIntent string

const (
	DocumentUploadIntent MediaUploadIntent = "document"

	// DocumentImageUploadIntent is used for images extracted from a parsed
	// document (e.g. Docling-embedded pictures/charts), stored under
	// document_images/ so they stay presigned-GET-able like any other
	// intent, even though the write itself goes through RawObjectStore.
	DocumentImageUploadIntent MediaUploadIntent = "document_image"
)

// ValidMediaUploadIntents returns a slice of all valid media upload intents.
func ValidMediaUploadIntents() []MediaUploadIntent {
	return []MediaUploadIntent{
		DocumentUploadIntent,
		DocumentImageUploadIntent,
	}
}

// ObjectPrefixForIntent returns the S3 object prefix for the given media upload intent.
func ObjectPrefixForIntent(intent MediaUploadIntent) string {
	return string(intent) + "s/"
}

// IsValidMediaUploadIntent checks if the provided intent is valid.
func IsValidMediaUploadIntent(intent string) bool {
	return slices.Contains(ValidMediaUploadIntents(), MediaUploadIntent(intent))
}

// S3ObjectInfo holds metadata about an S3 object.
type S3ObjectInfo struct {
	Key          string
	LastModified time.Time
}

// MediaStorageAdmin defines admin-level storage operations (list, delete).
// Separate from MediaUploadProvider to keep client-facing interface clean.
type MediaStorageAdmin interface {
	// ListObjects returns all objects under the given prefix.
	ListObjects(ctx context.Context, prefix string) ([]S3ObjectInfo, error)

	// DeleteObject removes the object with the given key.
	DeleteObject(ctx context.Context, key string) error
}

// RawObjectStore is internal, server-side S3 read/write — no presigned URL,
// no client-facing key validation. validation.ValidateS3PrefixAndFile exists
// to gate untrusted client input (a browser/CLI calling GetPresignedPOSTURL);
// keys used here are always server-generated and never handed to a client,
// so that validation doesn't apply.
type RawObjectStore interface {
	// PutObject writes size bytes from r to key, overwriting any existing object.
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// GetObject returns a reader for the object at key. Callers must close it.
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
}

// MediaUploadProvider defines the interface for media upload providers.
type MediaUploadProvider interface {
	// GetPresignedPOSTURL generates a presigned POST URL for uploading media.
	GetPresignedPOSTURL(
		ctx context.Context,
		intent MediaUploadIntent,
		filename, contentType string,
	) (*url.URL, map[string]string, error)

	// GetPresignedGETURL generates a presigned GET URL for accessing media.
	GetPresignedGETURL(
		ctx context.Context,
		s3Key string,
		expirySeconds int64,
	) (string, error)

	// GetPresignedGETURLs generates presigned GET URLs for multiple S3 keys concurrently.
	// Returns a map of s3Key -> presignedURL. Keys with errors are omitted from the result.
	GetPresignedGETURLs(
		ctx context.Context,
		s3Keys []string,
		expirySeconds int64,
	) (map[string]string, error)
}
