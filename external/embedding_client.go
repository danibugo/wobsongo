package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/kairosedubf/wobsongo/data"
)

// EmbeddingClient implements data.Embedder against a generic
// OpenAI-compatible embeddings API — works unmodified against self-hosted
// vLLM/text-embeddings-inference or any hosted provider using that shape.
type EmbeddingClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure EmbeddingClient implements data.Embedder.
var _ data.Embedder = (*EmbeddingClient)(nil)

// NewEmbeddingClient creates a new EmbeddingClient targeting the given base
// URL/model. apiKey may be empty — self-hosted servers often need no auth.
func NewEmbeddingClient(baseURL, model, apiKey string) *EmbeddingClient {
	return &EmbeddingClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// embeddingsRequest is the request payload for an OpenAI-compatible
// /v1/embeddings call.
type embeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingsResponse is the response payload from an OpenAI-compatible
// /v1/embeddings call.
type embeddingsResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed implements data.Embedder.
func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	payload := embeddingsRequest{
		Model: c.model,
		Input: texts,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embeddings request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/embeddings",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
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
			"embeddings endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed embeddingsResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embeddings response: %w", err)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf(
			"embeddings endpoint returned %d vectors for %d inputs",
			len(parsed.Data),
			len(texts),
		)
	}

	// Don't trust response ordering even though the spec generally
	// guarantees it — re-sort by the server-reported index.
	sort.Slice(parsed.Data, func(i, j int) bool {
		return parsed.Data[i].Index < parsed.Data[j].Index
	})

	vectors := make([][]float32, len(parsed.Data))
	for i := range parsed.Data {
		vectors[i] = parsed.Data[i].Embedding
	}
	return vectors, nil
}
