package validation

import (
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRegister(t *testing.T) {
	e := echo.New()
	if err := Register(e); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	if e.Validator == nil {
		t.Fatal("Register() did not set e.Validator")
	}
	if _, ok := e.Validator.(*DTOValidator); !ok {
		t.Fatalf("e.Validator is %T, want *DTOValidator", e.Validator)
	}
}

func TestDTOValidator_Validate(t *testing.T) {
	e := echo.New()
	if err := Register(e); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	type keyDTO struct {
		Key string `validate:"s3key"`
	}
	type mimeDTO struct {
		Mime string `validate:"mimetype"`
	}

	tests := []struct {
		name    string
		dto     any
		wantErr bool
	}{
		{"valid s3 key", keyDTO{Key: "documents/" + validHash + ".pdf"}, false},
		{"invalid s3 key", keyDTO{Key: "not-a-valid-key"}, true},
		{"valid mime type", mimeDTO{Mime: "image/png"}, false},
		{"invalid mime type", mimeDTO{Mime: "not-a-mime"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := e.Validator.Validate(tt.dto)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%+v): error = %v, wantErr %v", tt.dto, err, tt.wantErr)
			}
		})
	}
}
