package config

import "testing"

func TestIsPathStyleEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"localhost:9000", true},
		{"127.0.0.1:9000", true},
		{"192.168.1.1:9000", true},
		{"minio:9000", true}, // hostname with no dot, still treated as path-style
		{"nyc3.digitaloceanspaces.com", false},
		{"s3.amazonaws.com", false},
		{"one.two.three.four:9000", false}, // 4 dot-separated segments, but non-numeric
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := isPathStyleEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("isPathStyleEndpoint(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestS3Config_GetBaseURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  S3Config
		want string
	}{
		{
			name: "path-style over http",
			cfg:  S3Config{Endpoint: "localhost:9000", BucketName: "mybucket", UseSSL: false},
			want: "http://localhost:9000/mybucket/",
		},
		{
			name: "path-style over https",
			cfg:  S3Config{Endpoint: "127.0.0.1:9000", BucketName: "mybucket", UseSSL: true},
			want: "https://127.0.0.1:9000/mybucket/",
		},
		{
			name: "virtual-hosted-style over https",
			cfg:  S3Config{Endpoint: "nyc3.digitaloceanspaces.com", BucketName: "mybucket", UseSSL: true},
			want: "https://mybucket.nyc3.digitaloceanspaces.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetBaseURL(); got != tt.want {
				t.Errorf("GetBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewS3Config(t *testing.T) {
	t.Run("missing endpoint", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "")
		t.Setenv("S3_ACCESS_KEY", "key")
		t.Setenv("S3_SECRET", "secret")
		t.Setenv("S3_BUCKET_NAME", "bucket")
		if _, err := NewS3Config(); err == nil {
			t.Error("expected error for missing S3_ENDPOINT, got nil")
		}
	})

	t.Run("missing access key", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "localhost:9000")
		t.Setenv("S3_ACCESS_KEY", "")
		t.Setenv("S3_SECRET", "secret")
		t.Setenv("S3_BUCKET_NAME", "bucket")
		if _, err := NewS3Config(); err == nil {
			t.Error("expected error for missing S3_ACCESS_KEY, got nil")
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "localhost:9000")
		t.Setenv("S3_ACCESS_KEY", "key")
		t.Setenv("S3_SECRET", "")
		t.Setenv("S3_BUCKET_NAME", "bucket")
		if _, err := NewS3Config(); err == nil {
			t.Error("expected error for missing S3_SECRET, got nil")
		}
	})

	t.Run("missing bucket name", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "localhost:9000")
		t.Setenv("S3_ACCESS_KEY", "key")
		t.Setenv("S3_SECRET", "secret")
		t.Setenv("S3_BUCKET_NAME", "")
		if _, err := NewS3Config(); err == nil {
			t.Error("expected error for missing S3_BUCKET_NAME, got nil")
		}
	})

	t.Run("valid with defaults", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "localhost:9000")
		t.Setenv("S3_ACCESS_KEY", "key")
		t.Setenv("S3_SECRET", "secret")
		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("S3_REGION", "")
		t.Setenv("S3_USE_SSL", "")

		cfg, err := NewS3Config()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Region != "us-east-1" {
			t.Errorf("Region = %q, want default %q", cfg.Region, "us-east-1")
		}
		if !cfg.UseSSL {
			t.Error("UseSSL = false, want default true")
		}
	})

	t.Run("valid with overrides", func(t *testing.T) {
		t.Setenv("S3_ENDPOINT", "nyc3.digitaloceanspaces.com")
		t.Setenv("S3_ACCESS_KEY", "key")
		t.Setenv("S3_SECRET", "secret")
		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("S3_REGION", "nyc3")
		t.Setenv("S3_USE_SSL", "false")

		cfg, err := NewS3Config()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Region != "nyc3" {
			t.Errorf("Region = %q, want %q", cfg.Region, "nyc3")
		}
		if cfg.UseSSL {
			t.Error("UseSSL = true, want false")
		}
	})
}
