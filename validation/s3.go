package validation

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/kairosedubf/wobsongo/data"
)

// regexSHA256Filename ensures the filename is exactly 64 hex chars + valid extension.
var (
	regexSHA256Filename = regexp.MustCompile(
		`^[a-fA-F0-9]{64}\.(jpg|jpeg|png|webp|avif|pdf|docx|doc|rtf|html|md)$`,
	)
	pathTraversalRegex = regexp.MustCompile(`\.\.`)
)

// ValidateFilename checks if the filename part of a path is a valid SHA-256 hash with an file extension.
// It accepts full paths (e.g., "avatars/abc...def.jpg") or just filenames ("abc...def.jpg").
func ValidateFilename(path string) bool {
	// 1. Extract just the filename (e.g., "avatars/hash.jpg" -> "hash.jpg")
	filename := filepath.Base(path)

	// 2. Normalize Windows paths if necessary (though usually S3 keys use forward slashes)
	// If filepath.Base returns "." or "/", the input was likely empty or just a slash.
	if filename == "." || filename == "/" {
		return false
	}

	// 3. Match against the strictly defined regex
	return regexSHA256Filename.MatchString(filename)
}

// ValidateS3PrefixAndFile validates an S3 key format with a specific prefix and filename,
// returning true if valid, false otherwise.
func ValidateS3PrefixAndFile(key string) bool {
	// Reject empty strings
	if key == "" {
		return false
	}

	// Reject path traversal attempts
	if pathTraversalRegex.MatchString(key) {
		return false
	}

	split := strings.Split(key, "/")

	// Check if there's at least one folder (intent) and a filename
	if len(split) != 2 {
		return false
	}

	// Check if the intent part is valid
	for _, intent := range data.ValidMediaUploadIntents() {
		validPrefix := data.ObjectPrefixForIntent(intent)

		// Check if the key starts with the valid prefix
		if split[0]+"/" == validPrefix {
			// Validate the filename part
			return ValidateFilename(split[1])
		}
	}

	// Reject if no valid intent prefix matched
	return false
}

// validateS3Key validates a generic S3 key format, according to intents
// defined in the data package.
func validateS3Key(fl validator.FieldLevel) bool {
	return ValidateS3PrefixAndFile(fl.Field().String())
}
