// Package validation provides a custom validator for Echo
// that uses go-playground/validator to validate
// DTOs (Data Transfer Objects) in the application.
// It defines a DTOValidator struct that implements the echo.Validator
// interface and a Register function to set up the custom validator with an Echo instance.
// The Register function also calls other functions to register specific
// validation rules for authentication and HTML-related DTOs.
package validation

import (
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

// DTOValidator is the custom validator for Echo that uses go-playground/validator.
type DTOValidator struct {
	validator *validator.Validate
}

// Validate performs validation on the given struct.
// Satisfies the echo.Validator interface.
func (cv *DTOValidator) Validate(i any) error {
	return cv.validator.Struct(i)
}

// Register sets up the custom DTO validator with an Echo instance.
func Register(e *echo.Echo) error {
	v := validator.New()
	if err := v.RegisterValidation("s3key", validateS3Key); err != nil {
		return err
	}
	if err := v.RegisterValidation("mimetype", validateMimeType); err != nil {
		return err
	}
	e.Validator = &DTOValidator{validator: v}
	return nil
}
