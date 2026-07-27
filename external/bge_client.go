package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kairosedubf/wobsongo/data"
)

// BGEClient implements data.Embedder for self-hosted BGE deployments that
// expose a simple POST {baseURL} endpoint with {"texts": [...]} rather than
// the standard OpenAI-compatible POST {baseURL}/v1/embeddings shape. The
// response carries embeddings in input order — no index field to re-sort by.
type BGEClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Ensure BGEClient implements data.Embedder.
var _ data.Embedder = (*BGEClient)(nil)

// NewBGEClient creates a new BGEClient. apiKey is optional — self-hosted
// deployments often require no authentication.
func NewBGEClient(baseURL, apiKey string) *BGEClient {
	return &BGEClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

type bgeEmbedRequest struct {
	Texts []string `json:"texts"`
}

type bgeEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error"`
}

// Embed implements data.Embedder.
func (c *BGEClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(bgeEmbedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call embeddings endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read embeddings response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"embeddings endpoint returned status %d: %s",
			resp.StatusCode, string(respBytes),
		)
	}

	var parsed bgeEmbedResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embeddings response: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("embeddings endpoint returned error: %s", parsed.Error)
	}
	if len(parsed.Embeddings) != len(texts) {
		return nil, fmt.Errorf(
			"embeddings endpoint returned %d vectors for %d inputs",
			len(parsed.Embeddings), len(texts),
		)
	}

	return parsed.Embeddings, nil
}
