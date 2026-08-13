package validation

import "testing"

// validHash is a 64-char hex string, matching the SHA-256 filename format.
const validHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"valid filename only", validHash + ".pdf", true},
		{"valid with directory prefix", "documents/" + validHash + ".png", true},
		{"valid uppercase hex", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA.jpg", true},
		{"empty path", "", false},
		{"root slash", "/", false},
		{"hash too short", "abc123.jpg", false},
		{"disallowed extension", validHash + ".exe", false},
		{"missing extension", validHash, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateFilename(tt.path); got != tt.want {
				t.Errorf("ValidateFilename(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateS3PrefixAndFile(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"empty key", "", false},
		{"path traversal", "documents/../" + validHash + ".pdf", false},
		{"missing filename segment", "documents", false},
		{"too many segments", "documents/sub/" + validHash + ".pdf", false},
		{"unknown intent prefix", "unknown/" + validHash + ".pdf", false},
		{"valid document intent", "documents/" + validHash + ".pdf", true},
		{"valid document_image intent", "document_images/" + validHash + ".png", true},
		{"valid prefix, invalid filename", "documents/not-a-valid-hash.pdf", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateS3PrefixAndFile(tt.key); got != tt.want {
				t.Errorf("ValidateS3PrefixAndFile(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
