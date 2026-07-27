package config

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func getEnv(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseLogLevel(l int) slog.Level {
	switch l {
	case 0:
		return slog.LevelDebug
	case 1:
		return slog.LevelInfo
	case 2:
		return slog.LevelWarn
	case 3:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// GetBaseURL returns the base URL for this S3 configuration.
// Returns the URL pattern used for constructing full S3 object URLs.
func (s *S3Config) GetBaseURL() string {
	protocol := schemeHTTPS
	if !s.UseSSL {
		protocol = "http"
	}

	// Detect path-style vs virtual-hosted-style based on endpoint
	if isPathStyleEndpoint(s.Endpoint) {
		// Path-style: http://endpoint/bucket/
		return fmt.Sprintf("%s://%s/%s/", protocol, s.Endpoint, s.BucketName)
	}
	// Virtual-hosted-style: https://bucket.endpoint/
	return fmt.Sprintf("%s://%s.%s/", protocol, s.BucketName, s.Endpoint)
}

// isPathStyleEndpoint determines if the endpoint should use path-style URLs.
// Path-style is used for localhost, IP addresses, and MinIO-style endpoints.
func isPathStyleEndpoint(endpoint string) bool {
	// localhost or 127.0.0.1
	if strings.HasPrefix(endpoint, "localhost") || strings.HasPrefix(endpoint, "127.0.0.1") {
		return true
	}
	// IP address with port (e.g., "192.168.1.1:9000")
	if strings.Contains(endpoint, ":") && !strings.Contains(endpoint, ".") {
		return true
	}
	// Check if it's an IP address
	parts := strings.Split(endpoint, ":")
	if len(parts) > 0 {
		ip := parts[0]
		// Simple IP detection: 4 parts separated by dots, all numeric
		ipParts := strings.Split(ip, ".")
		if len(ipParts) == 4 {
			allNumeric := true
			for _, part := range ipParts {
				if _, err := strconv.Atoi(part); err != nil {
					allNumeric = false
					break
				}
			}
			if allNumeric {
				return true
			}
		}
	}
	return false
}

// loadEnvFile loads environment variables from a .env file.
func loadEnvFile(envs []string) {
	if len(envs) == 0 || envs[0] == "" {
		fmt.Println("(loadEnvFile) No .env file specified, skipping loading from file.")
		return
	}
	source := envs[0]
	err := godotenv.Load(source)
	if err != nil {
		fmt.Printf("Warning: Failed to load .env file: %s\n", err.Error())
	} else {
		fmt.Printf("Loaded environment from: %s\n", source)
	}
}

// createLogger creates a logger based on the environment and log level.
func createLogger(appEnv string, logLevel slog.Level) *slog.Logger {
	addSource := appEnv == DevelopmentEnv || appEnv == TestingEnv
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: addSource,
	}))
}

// parseJWTExpiryHours parses the JWT expiry hours from environment variable.
func parseJWTExpiryHours(logger *slog.Logger) int {
	appJwtExpiryHoursStr := getEnv("APP_JWT_EXPIRY_HOURS", "")
	if appJwtExpiryHoursStr == "" {
		logger.Error("APP_JWT_EXPIRY_HOURS is not set.")
	}
	jwtExpiryHours, err := strconv.Atoi(appJwtExpiryHoursStr)
	if err != nil {
		jwtExpiryHours = 7 * 24 // 7 days
	}
	return jwtExpiryHours
}

// parsePort parses the port from environment variable.
func parsePort(logger *slog.Logger) int {
	appPortStr := getEnv("APP_PORT", "8000")
	port, err := strconv.Atoi(appPortStr)
	if err != nil {
		logger.Error("Invalid port value", "port", appPortStr)
		port = 8000
	}
	return port
}

// parseCORSMethods parses CORS allowed methods from environment variable.
func parseCORSMethods() []string {
	corsAllowedMethodsStr := getEnv("CORS_ALLOWED_METHODS", "")
	if corsAllowedMethodsStr == "" {
		return []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPut,
			http.MethodPatch,
			http.MethodPost,
			http.MethodDelete,
		}
	}
	return strings.Split(corsAllowedMethodsStr, ",")
}

// loadS3ConfigOrDefault loads S3 configuration or returns default for testing.
func loadS3ConfigOrDefault(logger *slog.Logger, envs ...string) *S3Config {
	s3Config, err := NewS3Config(envs...)
	if err != nil {
		logger.Warn("Failed to load S3 configuration, using defaults for tests", "error", err)
		return &S3Config{
			Endpoint:   "mock://localhost:9000",
			Region:     "us-east-1",
			AccessKey:  "test",
			SecretKey:  "test",
			BucketName: "test-bucket",
			UseSSL:     false,
		}
	}
	return s3Config
}

// loadVLMConfigOrDefault loads VLM configuration, or an empty VLMConfig if
// unset/invalid. Deliberately empty (not a fake populated default like
// loadS3ConfigOrDefault) — config.IsVLMOK is a hard startup requirement in
// cmd/server.go, and a fake non-empty default would defeat that check.
func loadVLMConfigOrDefault(logger *slog.Logger, envs ...string) *VLMConfig {
	vlmConfig, err := NewVLMConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load VLM configuration", "error", err)
		return &VLMConfig{}
	}
	return vlmConfig
}

// loadEmbeddingConfigOrDefault loads Embedding configuration, or an empty
// EmbeddingConfig if unset/invalid. Deliberately empty (not a fake populated
// default) — config.IsEmbeddingOK is a hard startup requirement in
// cmd/server.go, and a fake non-empty default would defeat that check.
func loadEmbeddingConfigOrDefault(logger *slog.Logger, envs ...string) *EmbeddingConfig {
	embeddingConfig, err := NewEmbeddingConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load Embedding configuration", "error", err)
		return &EmbeddingConfig{}
	}
	return embeddingConfig
}

// loadExtractionConfigOrDefault loads Extraction configuration, or an empty
// ExtractionConfig if unset/invalid. Deliberately empty (not a fake populated
// default) — config.IsExtractionOK is a hard startup requirement in
// cmd/server.go, and a fake non-empty default would defeat that check.
func loadExtractionConfigOrDefault(logger *slog.Logger, envs ...string) *ExtractionConfig {
	extractionConfig, err := NewExtractionConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load Extraction configuration", "error", err)
		return &ExtractionConfig{}
	}
	return extractionConfig
}

// loadTranslationConfigOrDefault loads Translation configuration, or an empty
// TranslationConfig if unset/invalid. Deliberately empty (not a fake
// populated default) — config.IsTranslationOK is a hard startup
// requirement wherever the translation worker is actually wired in, and a
// fake non-empty default would defeat that check.
func loadTranslationConfigOrDefault(logger *slog.Logger, envs ...string) *TranslationConfig {
	translationConfig, err := NewTranslationConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load Translation configuration", "error", err)
		return &TranslationConfig{}
	}
	return translationConfig
}

// loadClaimCheckConfigOrDefault loads ClaimCheck configuration, or an empty
// ClaimCheckConfig if unset/invalid. Deliberately empty (not a fake populated
// default) — config.IsClaimCheckOK is a hard startup requirement wherever
// the claim-checking feature is actually wired in, and a fake non-empty
// default would defeat that check.
func loadClaimCheckConfigOrDefault(logger *slog.Logger, envs ...string) *ClaimCheckConfig {
	claimCheckConfig, err := NewClaimCheckConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load ClaimCheck configuration", "error", err)
		return &ClaimCheckConfig{}
	}
	return claimCheckConfig
}

// loadEmailConfigOrDefault loads email configuration or returns default for testing.
func loadEmailConfigOrDefault(logger *slog.Logger, envs ...string) *EmailConfig {
	emailConfig, err := NewEmailConfig(envs...)
	if err != nil {
		logger.Warn("Failed to load Email configuration, using mock provider", "error", err)
		return &EmailConfig{
			Transactional: TransactionalEmailConfig{
				Provider:    ProviderMock,
				FromName:    "Planet League",
				FromAddress: "noreply@planetleague.com",
			},
		}
	}
	return emailConfig
}
