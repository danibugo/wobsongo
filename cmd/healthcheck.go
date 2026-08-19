package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/spf13/cobra"
)

// healthcheckPixelPNG is a 1x1 transparent PNG, used as the minimal valid
// image for the VLM caption health check — avoids embedding raw binary in
// source while still exercising the real Caption() code path.
const healthcheckPixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// healthcheckDefaultTimeout bounds each individual reachability check by
// default — a totally unreachable host shouldn't hang the whole command.
// Overridable via --timeout since scale-to-zero hosts (e.g. Modal) can take
// well past this on a cold request.
const healthcheckDefaultTimeout = 30 * time.Second

// healthcheckTimeout is set from the --timeout flag in init().
var healthcheckTimeout time.Duration

var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Verify required configuration is present and backing services are reachable",
	Long: "Checks every config gate cmd/server.go enforces at startup (Postgres, S3,\n" +
		"VLM, Embedding, Extraction), then actually exercises each configured\n" +
		"service using the app's own client code: Postgres ping, S3 bucket check,\n" +
		"Docling GET /health, a real (tiny) VLM caption call, a real embedding\n" +
		"call, and a real extraction call. Exits non-zero if anything is missing\n" +
		"or unreachable.\n\n" +
		"Deliberately doesn't probe GET /v1/models — plenty of self-hosted\n" +
		"OpenAI-compatible servers don't implement that discovery endpoint even\n" +
		"though their actual chat/embeddings endpoints work fine, which would\n" +
		"make it an unreliable health signal.\n\n" +
		"Increase --timeout for scale-to-zero hosts (e.g. Modal) — a cold\n" +
		"container can easily take well past the default before it responds.",
	Run: runHealthcheck,
}

func init() {
	healthcheckCmd.Flags().DurationVar(
		&healthcheckTimeout,
		"timeout",
		healthcheckDefaultTimeout,
		"Per-check timeout (e.g. 60s, 2m) — increase for cold-starting services",
	)
}

// healthcheckResult is one row of output: a labeled check that either
// passed, failed with an error, or was skipped (usually because its config
// prerequisite already failed).
type healthcheckResult struct {
	label   string
	err     error
	skipped bool
}

func runHealthcheck(cmd *cobra.Command, _ []string) {
	cfg := appconfig.NewConfig(EnvFile)
	ctx := cmd.Context()

	var results []healthcheckResult
	check := func(label string, err error) {
		results = append(results, healthcheckResult{label: label, err: err})
	}
	skip := func(label string) {
		results = append(results, healthcheckResult{label: label, skipped: true})
	}

	cmd.Println("=== Configuration ===")
	coreErr := cfg.IsOK()
	check(
		"Core config (APP_DB_URI, APP_JWT_SECRET, APP_JWT_EXPIRY_HOURS, EmailConfig, Port)",
		coreErr,
	)

	s3Err := appconfig.IsS3OK(cfg.S3Config)
	check("S3 config", s3Err)

	vlmErr := appconfig.IsVLMOK(cfg.VLMConfig)
	check("VLM config", vlmErr)

	embeddingErr := appconfig.IsEmbeddingOK(cfg.EmbeddingConfig)
	check("Embedding config", embeddingErr)

	extractionErr := appconfig.IsExtractionOK(cfg.ExtractionConfig)
	check("Extraction config", extractionErr)

	printResults(cmd, results)
	results = nil

	cmd.Println("\n=== Connectivity ===")

	if cfg.PostgresURI == "" {
		skip("Postgres (ping)")
	} else {
		check("Postgres (ping)", checkPostgres(ctx, cfg.PostgresURI))
	}

	if s3Err != nil {
		skip("S3 (bucket check)")
	} else {
		check("S3 (bucket check)", checkS3(ctx, cfg.S3Config))
	}

	if cfg.DoclingBaseURL == "" {
		skip("Docling (GET /health)")
	} else {
		check("Docling (GET /health)", checkHTTPHealth(ctx, cfg.DoclingBaseURL+"/health", ""))
	}

	if vlmErr != nil {
		skip("VLM (test caption call)")
	} else {
		check("VLM (test caption call)", checkVLM(ctx, cfg.VLMConfig))
	}

	if embeddingErr != nil {
		skip("Embedding (test embed call)")
	} else {
		check("Embedding (test embed call)", checkEmbedding(ctx, cfg.EmbeddingConfig))
	}

	if extractionErr != nil {
		skip("Extraction (test extraction call)")
	} else {
		check("Extraction (test extraction call)", checkExtraction(ctx, cfg.ExtractionConfig))
	}

	failed := printResults(cmd, results)
	if failed > 0 {
		cmd.Printf("\n%d check(s) failed.\n", failed)
		os.Exit(1)
	}
	cmd.Println("\nAll checks passed.")
}

// printResults prints each result as a PASS/FAIL/SKIP line and returns the
// number of failures.
func printResults(cmd *cobra.Command, results []healthcheckResult) int {
	failed := 0
	for _, r := range results {
		switch {
		case r.skipped:
			cmd.Printf("  [SKIP] %s\n", r.label)
		case r.err != nil:
			failed++
			cmd.Printf("  [FAIL] %s: %s\n", r.label, r.err.Error())
		default:
			cmd.Printf("  [OK]   %s\n", r.label)
		}
	}
	return failed
}

// checkPostgres opens a pool (with pgvector types registered, matching every
// real entrypoint) and pings it.
func checkPostgres(ctx context.Context, uri string) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	pool, err := repo.NewPgxPool(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}
	return nil
}

// checkS3 reuses NewS3Provider's own bucket-existence check (the same
// construction path cmd/server.go and cmd/document_insert.go already rely on).
func checkS3(ctx context.Context, cfg *appconfig.S3Config) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	if _, err := repo.NewS3Provider(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// checkHTTPHealth issues a GET against url (optionally authenticated) and
// treats any 2xx response as healthy.
func checkHTTPHealth(ctx context.Context, url, apiKey string) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("returned status %d", resp.StatusCode)
	}
	return nil
}

// checkVLM exercises the real Caption() code path with a minimal 1x1 pixel
// image — more reliable than probing GET /v1/models, which many self-hosted
// vision servers don't implement even when captioning itself works fine.
func checkVLM(ctx context.Context, cfg *appconfig.VLMConfig) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	imageBytes, err := base64.StdEncoding.DecodeString(healthcheckPixelPNG)
	if err != nil {
		return fmt.Errorf("failed to decode test image: %w", err)
	}

	client := external.NewVLMClient(cfg.BaseURL, cfg.Model, cfg.APIKey)
	if _, err := client.Caption(ctx, &data.CaptionRequest{
		ImageBytes:  imageBytes,
		ContentType: "image/png",
	}); err != nil {
		return err
	}
	return nil
}

// checkEmbedding exercises the real Embed() code path with a trivial string,
// using whichever client cfg.Provider selects (same helper cmd/server.go uses).
func checkEmbedding(ctx context.Context, cfg *appconfig.EmbeddingConfig) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	client := newEmbeddingClient(cfg)
	if _, err := client.Embed(ctx, []string{"healthcheck"}); err != nil {
		return err
	}
	return nil
}

// checkExtraction exercises the real Extract() code path with a trivial
// sentence. Its content doesn't matter — zero extracted facts is still a
// successful, healthy response; only a transport/HTTP error counts as failure.
func checkExtraction(ctx context.Context, cfg *appconfig.ExtractionConfig) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	client := external.NewExtractionClient(cfg.BaseURL, cfg.Model, cfg.APIKey)
	if _, err := client.Extract(
		ctx,
		&data.ExtractionRequest{Text: "The sky is blue."},
	); err != nil {
		return err
	}
	return nil
}
