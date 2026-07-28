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
	"github.com/kairosedubf/wobsongo/model"
)

// translationMaxTokens bounds the LLM's response length for one translation
// call. Chunks can run long, so this is sized well above extractionMaxTokens
// (which bounds a compact JSON fact list, not free-form prose).
const translationMaxTokens = 8000

// translationHTTPTimeout bounds a single translation call, mirroring
// extractionHTTPTimeout's reasoning — must stay comfortably below whatever
// per-item budget TranslateChunksWorker allots.
const translationHTTPTimeout = 7 * time.Minute

// translationChatRoleUser is the chat-completion message role for the caller's input.
const translationChatRoleUser = "user"

// TranslationClient implements data.Translator against a generic
// OpenAI-compatible text chat-completions API — works unmodified against
// self-hosted vLLM/Ollama or any hosted provider using that shape.
type TranslationClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure TranslationClient implements data.Translator.
var _ data.Translator = (*TranslationClient)(nil)

// NewTranslationClient creates a new TranslationClient targeting the given
// base URL/model. apiKey may be empty — self-hosted servers often need no auth.
func NewTranslationClient(baseURL, model, apiKey string) *TranslationClient {
	return &TranslationClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: translationHTTPTimeout,
		},
	}
}

// Translate implements data.Translator.
func (c *TranslationClient) Translate(
	ctx context.Context,
	text string,
	sourceLanguage model.Language,
) (string, error) {
	payload := extractionCompletionRequest{
		Model: c.model,
		Messages: []extractionChatMessage{
			{Role: translationChatRoleUser, Content: buildTranslationPrompt(text, sourceLanguage)},
		},
		MaxTokens: translationMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal translation request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create translation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call translation endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read translation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"translation endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("failed to unmarshal translation response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("translation response contained no choices: %s", respBytes)
	}
	if parsed.Choices[0].FinishReason == "length" {
		return "", fmt.Errorf(
			"translation response was truncated by max_tokens=%d before finishing",
			translationMaxTokens,
		)
	}

	return stripJSONCodeFence(parsed.Choices[0].Message.Content), nil
}

// languageDisplayNames gives the LLM prompt an unambiguous, human-readable
// language name rather than relying on it to interpret "en"/"fr" ISO codes.
var languageDisplayNames = map[model.Language]string{
	model.LanguageEnglish: "English",
	model.LanguageFrench:  "French",
}

// buildTranslationPrompt builds the translation instruction for text, going
// from sourceLanguage into the other supported language.
func buildTranslationPrompt(text string, sourceLanguage model.Language) string {
	targetLanguage := sourceLanguage.Other()
	return fmt.Sprintf(
		"Translate the following text from %s to %s. "+
			"Respond with ONLY the translated text (no markdown, no commentary, "+
			"no repetition of the source text).\n\nText:\n\"\"\"\n%s\n\"\"\"\n",
		languageDisplayNames[sourceLanguage],
		languageDisplayNames[targetLanguage],
		text,
	)
}
