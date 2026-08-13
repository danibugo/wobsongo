package validation

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestValidateMimeType(t *testing.T) {
	v := validator.New()
	if err := v.RegisterValidation("mimetype", validateMimeType); err != nil {
		t.Fatalf("failed to register mimetype validation: %v", err)
	}

	type dto struct {
		Mime string `validate:"mimetype"`
	}

	tests := []struct {
		name    string
		mime    string
		wantErr bool
	}{
		{"valid image type", "image/png", false},
		{"valid type with plus", "application/vnd.api+json", false},
		{"valid type with dot and dash", "application/vnd.ms-excel", false},
		{"missing subtype", "image", true},
		{"empty string", "", true},
		{"missing type", "/png", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(dto{Mime: tt.mime})
			if (err != nil) != tt.wantErr {
				t.Errorf("validate mimetype %q: error = %v, wantErr %v", tt.mime, err, tt.wantErr)
			}
		})
	}
}
