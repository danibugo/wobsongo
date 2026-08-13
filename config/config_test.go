package config

import "testing"

func TestIsS3OK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *S3Config
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing endpoint", &S3Config{AccessKey: "a", SecretKey: "s", BucketName: "b"}, true},
		{"missing access key", &S3Config{Endpoint: "e", SecretKey: "s", BucketName: "b"}, true},
		{"missing secret key", &S3Config{Endpoint: "e", AccessKey: "a", BucketName: "b"}, true},
		{"missing bucket name", &S3Config{Endpoint: "e", AccessKey: "a", SecretKey: "s"}, true},
		{"complete", &S3Config{Endpoint: "e", AccessKey: "a", SecretKey: "s", BucketName: "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsS3OK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsS3OK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsVLMOK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *VLMConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base url", &VLMConfig{Model: "m"}, true},
		{"missing model", &VLMConfig{BaseURL: "u"}, true},
		{"complete", &VLMConfig{BaseURL: "u", Model: "m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsVLMOK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsVLMOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsEmbeddingOK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *EmbeddingConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base url", &EmbeddingConfig{Model: "m", Provider: EmbeddingProviderOpenAI}, true},
		{"missing model", &EmbeddingConfig{BaseURL: "u", Provider: EmbeddingProviderOpenAI}, true},
		{"unrecognized provider", &EmbeddingConfig{BaseURL: "u", Model: "m", Provider: "bogus"}, true},
		{"valid openai", &EmbeddingConfig{BaseURL: "u", Model: "m", Provider: EmbeddingProviderOpenAI}, false},
		{"valid bge", &EmbeddingConfig{BaseURL: "u", Model: "m", Provider: EmbeddingProviderBGE}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsEmbeddingOK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsEmbeddingOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsExtractionOK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ExtractionConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base url", &ExtractionConfig{Model: "m"}, true},
		{"missing model", &ExtractionConfig{BaseURL: "u"}, true},
		{"complete", &ExtractionConfig{BaseURL: "u", Model: "m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsExtractionOK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsExtractionOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsTranslationOK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *TranslationConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base url", &TranslationConfig{Model: "m"}, true},
		{"missing model", &TranslationConfig{BaseURL: "u"}, true},
		{"complete", &TranslationConfig{BaseURL: "u", Model: "m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsTranslationOK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsTranslationOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsClaimCheckOK(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ClaimCheckConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base url", &ClaimCheckConfig{Model: "m"}, true},
		{"missing model", &ClaimCheckConfig{BaseURL: "u"}, true},
		{"complete", &ClaimCheckConfig{BaseURL: "u", Model: "m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsClaimCheckOK(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsClaimCheckOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func validConfig() *Config {
	return &Config{
		PostgresURI:    "postgres://x",
		JWTSecret:      "secret",
		EmailConfig:    &EmailConfig{},
		Port:           8000,
		JWTExpiryHours: 24,
	}
}

func TestConfig_IsOK(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr bool
	}{
		{"valid", func(c *Config) {}, false},
		{"missing postgres uri", func(c *Config) { c.PostgresURI = "" }, true},
		{"missing jwt secret", func(c *Config) { c.JWTSecret = "" }, true},
		{"missing email config", func(c *Config) { c.EmailConfig = nil }, true},
		{"port zero", func(c *Config) { c.Port = 0 }, true},
		{"port negative", func(c *Config) { c.Port = -1 }, true},
		{"port too large", func(c *Config) { c.Port = 70000 }, true},
		{"jwt expiry zero", func(c *Config) { c.JWTExpiryHours = 0 }, true},
		{"jwt expiry negative", func(c *Config) { c.JWTExpiryHours = -1 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(c)
			err := c.IsOK()
			if (err != nil) != tt.wantErr {
				t.Errorf("IsOK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_IsLocal(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{DevelopmentEnv, true},
		{TestingEnv, true},
		{StagingEnv, false},
		{ProductionEnv, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			c := &Config{Env: tt.env}
			if got := c.IsLocal(); got != tt.want {
				t.Errorf("IsLocal() for env %q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestConfig_APISchemes(t *testing.T) {
	tests := []struct {
		name    string
		apiHost string
		want    string
	}{
		{"localhost", "localhost:8000", "http"},
		{"remote host", "api.example.com", "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{APIHost: tt.apiHost}
			schemes := c.APISchemes()
			if len(schemes) != 1 || schemes[0] != tt.want {
				t.Errorf("APISchemes() for host %q = %v, want [%v]", tt.apiHost, schemes, tt.want)
			}
		})
	}
}
