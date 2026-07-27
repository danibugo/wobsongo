package external

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kairosedubf/wobsongo/data"
)

// captionMaxTokens bounds the VLM's response length for a single caption.
const captionMaxTokens = 500

// VLMClient implements data.ImageCaptioner against a generic
// OpenAI-compatible vision chat-completions API — works unmodified against
// self-hosted vLLM/Ollama or any hosted open-weight-model provider using
// that shape.
type VLMClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure VLMClient implements data.ImageCaptioner.
var _ data.ImageCaptioner = (*VLMClient)(nil)

// vlmHTTPTimeout bounds a single captioning call. Matches
// extractionHTTPTimeout's reasoning (external/extraction_client.go): a
// cloud-hosted model can genuinely take longer than 2 minutes under variable
// load, and captioning hit the same "context deadline exceeded" symptom
// against this same provider — must stay comfortably below
// captionPerChunkBudget (internal/worker/caption_image_chunks.go).
const vlmHTTPTimeout = 5 * time.Minute

// NewVLMClient creates a new VLMClient targeting the given base URL/model.
// apiKey may be empty — self-hosted servers often need no auth.
func NewVLMClient(baseURL, model, apiKey string) *VLMClient {
	return &VLMClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: vlmHTTPTimeout,
		},
	}
}

// chatCompletionRequest is the request payload for an OpenAI-compatible
// /v1/chat/completions vision call.
type chatCompletionRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string            `json:"role"`
	Content []chatContentPart `json:"content"`
}

type chatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// chatCompletionResponse is the response payload from an OpenAI-compatible
// /v1/chat/completions call.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		// FinishReason is "length" when the model was cut off by max_tokens
		// rather than finishing naturally — checked by ExtractionClient to
		// distinguish a too-small token budget from genuinely malformed JSON.
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Caption implements data.ImageCaptioner.
func (c *VLMClient) Caption(ctx context.Context, req *data.CaptionRequest) (string, error) {
	dataURL := fmt.Sprintf(
		"data:%s;base64,%s",
		req.ContentType,
		base64.StdEncoding.EncodeToString(req.ImageBytes),
	)

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role: "user",
				Content: []chatContentPart{
					{Type: "text", Text: buildPrompt(req)},
					{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
				},
			},
		},
		MaxTokens: captionMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VLM request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create VLM request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call VLM endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read VLM response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"VLM endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("failed to unmarshal VLM response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("VLM response contained no choices: %s", respBytes)
	}

	caption := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if caption == "" {
		return "", errors.New("VLM response contained an empty caption")
	}
	return caption, nil
}

// buildPrompt builds the captioning instruction, grounded with whatever
// document context is available. Covers both photos/diagrams and
// charts/graphs in one template rather than branching on layout type.
func buildPrompt(req *data.CaptionRequest) string {
	var b strings.Builder
	b.WriteString("You are generating a detailed, factual description of an image ")
	b.WriteString("extracted from a document, for storage in a searchable knowledge base ")
	b.WriteString("(plain-text and semantic search). Describe exactly what is shown: ")
	b.WriteString("objects, people, diagrams, or photos. If it is a chart or graph, ")
	b.WriteString("describe the axis labels, legend, and general data trend. Transcribe ")
	b.WriteString("any visible text verbatim. Be specific and detailed (3-6 sentences). ")
	b.WriteString("Do not speculate beyond what is visibly shown.\n\n")

	if req.DocumentTitle != "" {
		fmt.Fprintf(&b, "Document: %q\n", req.DocumentTitle)
	}
	if req.Page > 0 {
		fmt.Fprintf(&b, "Page: %d\n", req.Page)
	}
	if req.SurroundingText != "" {
		b.WriteString("Surrounding text on this page:\n\"\"\"\n")
		b.WriteString(req.SurroundingText)
		b.WriteString("\n\"\"\"\n")
	}

	return b.String()
}
