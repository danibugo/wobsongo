package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// extractionMaxTokens bounds the LLM's response length for one chunk's
// extracted facts. A chunk can yield dozens of facts (e.g. a personnel/
// affiliation roster), each needing ~40-80 tokens of JSON — confirmed against
// real responses that got cut off mid-fact at both the original 1500-token
// budget and, later, at 4000 (a personnel-roster-heavy chunk still exceeded
// it), producing invalid truncated JSON on every retry. Bumped alongside
// extractionHTTPTimeout (this file) and extractKnowledgePerChunkBudget
// (internal/worker/extract_knowledge.go) — a bigger budget takes longer to
// generate, so raising this alone would just trade truncation failures for
// more frequent timeout failures.
const extractionMaxTokens = 6000

// ExtractionClient implements data.KnowledgeExtractor against a generic
// OpenAI-compatible text chat-completions API — works unmodified against
// self-hosted vLLM/Ollama or any hosted provider using that shape.
type ExtractionClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// Ensure ExtractionClient implements data.KnowledgeExtractor.
var _ data.KnowledgeExtractor = (*ExtractionClient)(nil)

// extractionHTTPTimeout bounds a single extraction call. Confirmed against a
// real "Client.Timeout exceeded while awaiting headers" failure that the
// previous 2-minute budget wasn't enough for a cloud-hosted LLM generating up
// to extractionMaxTokens under variable load — must stay comfortably below
// extractKnowledgePerChunkBudget (internal/worker/extract_knowledge.go).
const extractionHTTPTimeout = 7 * time.Minute

// NewExtractionClient creates a new ExtractionClient targeting the given
// base URL/model. apiKey may be empty — self-hosted servers often need no auth.
func NewExtractionClient(baseURL, model, apiKey string) *ExtractionClient {
	return &ExtractionClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: extractionHTTPTimeout,
		},
	}
}

// extractionCompletionRequest is the request payload for an OpenAI-compatible
// text-only /v1/chat/completions call — unlike VLMClient's, Content is a
// plain string (no image parts).
type extractionCompletionRequest struct {
	Model     string                  `json:"model"`
	Messages  []extractionChatMessage `json:"messages"`
	MaxTokens int                     `json:"max_tokens"`
}

type extractionChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// extractedFactJSON is the wire shape the LLM is instructed to respond with,
// one per extracted fact.
type extractedFactJSON struct {
	Subject              string   `json:"subject"`
	Predicate            string   `json:"predicate"`
	Object               string   `json:"object"`
	TruthTier            string   `json:"truth_tier"`
	Category             string   `json:"category"`
	Topics               []string `json:"topics"`
	Note                 string   `json:"note"`
	SearchTextTranslated string   `json:"search_text_translated"`
}

// Extract implements data.KnowledgeExtractor.
func (c *ExtractionClient) Extract(
	ctx context.Context,
	req *data.ExtractionRequest,
) ([]data.ExtractedFact, error) {
	payload := extractionCompletionRequest{
		Model: c.model,
		Messages: []extractionChatMessage{
			{Role: "user", Content: buildExtractionPrompt(req)},
		},
		MaxTokens: extractionMaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal extraction request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create extraction request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call extraction endpoint: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read extraction response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"extraction endpoint returned error status: %d. Body: %s",
			resp.StatusCode,
			string(respBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extraction response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("extraction response contained no choices: %s", respBytes)
	}
	if parsed.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf(
			"extraction response was truncated by max_tokens=%d before finishing — "+
				"this chunk yields more facts than the budget allows",
			extractionMaxTokens,
		)
	}

	content := stripJSONCodeFence(parsed.Choices[0].Message.Content)
	if content == "" || content == "[]" {
		return nil, nil
	}

	var rawFacts []extractedFactJSON
	if err := json.Unmarshal([]byte(content), &rawFacts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extracted facts JSON: %w: %s", err, content)
	}

	facts := make([]data.ExtractedFact, 0, len(rawFacts))
	for _, raw := range rawFacts {
		tier, err := model.ParseTruthTier(raw.TruthTier)
		if err != nil {
			log.Printf(
				"[ExtractionClient] skipping fact with unrecognized truth_tier %q: subject=%q predicate=%q object=%q",
				raw.TruthTier,
				raw.Subject,
				raw.Predicate,
				raw.Object,
			)
			continue
		}
		category, err := model.ParseFactCategory(raw.Category)
		if err != nil {
			log.Printf(
				"[ExtractionClient] skipping fact with unrecognized category %q: subject=%q predicate=%q object=%q",
				raw.Category,
				raw.Subject,
				raw.Predicate,
				raw.Object,
			)
			continue
		}
		facts = append(facts, data.ExtractedFact{
			Subject:              raw.Subject,
			Predicate:            raw.Predicate,
			Object:               raw.Object,
			Note:                 raw.Note,
			TruthTier:            tier,
			Category:             category,
			Topics:               raw.Topics,
			TranslatedSearchText: raw.SearchTextTranslated,
		})
	}
	return facts, nil
}

// buildExtractionPrompt builds the fact-extraction instruction, grounded
// with whatever document context is available.
func buildExtractionPrompt(req *data.ExtractionRequest) string {
	var b strings.Builder
	b.WriteString("You are extracting atomic, verifiable facts from a chunk of text ")
	b.WriteString("extracted from a document, for storage in a structured knowledge base. ")
	b.WriteString("Break the text down into discrete subject-predicate-object facts. ")
	b.WriteString("Respond with ONLY a JSON array (no markdown, no commentary), where each ")
	b.WriteString("element has this shape:\n")
	b.WriteString(`{"subject": "...", "predicate": "...", "object": "...", ` +
		`"truth_tier": "...", "category": "...", "topics": ["..."], "note": "...", ` +
		`"search_text_translated": "..."}` + "\n\n")
	b.WriteString("truth_tier must be exactly one of: axiomatic, temporal, probabilistic, ")
	b.WriteString("subjective, unknown, invalid.\n")
	b.WriteString("- axiomatic: high factual accuracy and reliability.\n")
	b.WriteString("- temporal: based on observable evidence but may no longer hold true.\n")
	b.WriteString("- probabilistic: context-dependent, subject to change.\n")
	b.WriteString("- subjective: opinion or perspective, not strongly factual.\n")
	b.WriteString("- unknown: needs further verification or context.\n")
	b.WriteString("- invalid: false or misleading.\n\n")
	b.WriteString("category must be exactly one of: clinical, metadata, unknown.\n")
	b.WriteString("- clinical: a substantive clinical/scientific claim, finding, or ")
	b.WriteString("recommendation (treatment efficacy, diagnostic criteria, epidemiological ")
	b.WriteString("findings, guideline recommendations).\n")
	b.WriteString("- metadata: about the document itself, not clinical content — authorship, ")
	b.WriteString("affiliations, citations/references, guideline-development process, document ")
	b.WriteString(`structure (e.g. "Chapter 5 contains recommendations on..."), or other `)
	b.WriteString("administrative/bibliographic information.\n")
	b.WriteString("- unknown: genuinely unclear which.\n\n")
	b.WriteString("topics is a short list of subject-matter tags. note is optional context; ")
	b.WriteString("use \"\" if none. If the text contains no extractable factual claims, ")
	b.WriteString("respond with an empty array: [].\n\n")

	sourceLanguage := languageDisplayNames[req.Language]
	targetLanguage := languageDisplayNames[req.Language.Other()]
	fmt.Fprintf(
		&b,
		"The source text below is written in %s. Keep subject, predicate, ",
		sourceLanguage,
	)
	b.WriteString("object, and note in that SAME language — never translate them.\n")
	fmt.Fprintf(
		&b,
		"search_text_translated must be a %s translation of the concatenated "+
			"subject, predicate, object, and note — used only for cross-language "+
			"search, not for display.\n\n",
		targetLanguage,
	)

	if req.DocumentTitle != "" {
		fmt.Fprintf(&b, "Document: %q\n", req.DocumentTitle)
	}
	if req.PublisherName != "" {
		fmt.Fprintf(&b, "Publisher: %q", req.PublisherName)
		if req.PublicationYear > 0 {
			fmt.Fprintf(&b, " (%d)", req.PublicationYear)
		}
		b.WriteString("\n")
	}
	b.WriteString("Text:\n\"\"\"\n")
	b.WriteString(req.Text)
	b.WriteString("\n\"\"\"\n")

	return b.String()
}

// stripJSONCodeFence removes a leading/trailing markdown code fence (with or
// without a "json" language tag) around content, if present, and trims
// whitespace. Self-hosted OpenAI-compatible servers commonly wrap JSON
// output in a fence even when explicitly told not to.
func stripJSONCodeFence(content string) string {
	s := strings.TrimSpace(content)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimPrefix(s, "json")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
