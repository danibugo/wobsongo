package validation

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

var mimeTypeRegex = regexp.MustCompile(`^[a-zA-Z0-9]+/[a-zA-Z0-9\-.+]+$`)

// validateMimeType validates if the mime type is well-formed (e.g. "image/png").
func validateMimeType(fl validator.FieldLevel) bool {
	return mimeTypeRegex.MatchString(fl.Field().String())
}
