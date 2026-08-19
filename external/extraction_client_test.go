package external_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

// chatResponseBody builds a well-formed OpenAI-compatible chat-completions
// response body with content as the assistant message's content, letting
// encoding/json handle escaping (avoids fragile hand-escaped JSON strings).
func chatResponseBody(t *testing.T, content string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake chat response: %v", err)
	}
	return body
}

func TestExtractionClient_Extract_Success(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any

	factsJSON, err := json.Marshal([]map[string]any{
		{
			"subject": "Alice", "predicate": "founded", "object": "Acme",
			"truth_tier": "axiomatic", "category": "clinical",
			"topics": []string{"business"}, "note": "",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake facts: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, string(factsJSON)))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "llama-3.1-70b-instruct", "test-api-key")

	facts, err := client.Extract(t.Context(), &data.ExtractionRequest{
		Text:          "Alice founded Acme in 2020.",
		DocumentTitle: "Company History",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Subject != "Alice" || facts[0].Predicate != "founded" || facts[0].Object != "Acme" {
		t.Errorf("unexpected fact: %+v", facts[0])
	}
	if facts[0].TruthTier != model.TruthTierAxiomatic {
		t.Errorf("expected TruthTierAxiomatic, got %v", facts[0].TruthTier)
	}
	if len(facts[0].Topics) != 1 || facts[0].Topics[0] != "business" {
		t.Errorf("expected topics [business], got %v", facts[0].Topics)
	}
	if facts[0].Category != model.FactCategoryClinical {
		t.Errorf("expected FactCategoryClinical, got %v", facts[0].Category)
	}

	if gotPath != "/v1/chat/completions" {
		t.Errorf("expected path /v1/chat/completions, got %s", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer test-api-key", gotAuth)
	}
	if gotBody["model"] != "llama-3.1-70b-instruct" {
		t.Errorf("expected model %q, got %v", "llama-3.1-70b-instruct", gotBody["model"])
	}

	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected exactly 1 message, got %v", gotBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message to be an object, got %T", messages[0])
	}
	content, _ := message["content"].(string)
	if !strings.Contains(content, "Company History") ||
		!strings.Contains(content, "Alice founded Acme") {
		t.Errorf("expected prompt to include document title and chunk text, got: %s", content)
	}
}

func TestExtractionClient_Extract_StripsCodeFence(t *testing.T) {
	fenced := "```json\n" +
		`[{"subject":"a","predicate":"b","object":"c","truth_tier":"unknown","category":"clinical","topics":[],"note":""}]` +
		"\n```"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, fenced))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	facts, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 1 || facts[0].Subject != "a" {
		t.Errorf("expected 1 fact with subject 'a' after fence stripping, got %+v", facts)
	}
}

func TestExtractionClient_Extract_EmptyArray_NoFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, "[]"))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	facts, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "The sky is blue."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestExtractionClient_Extract_SkipsUnrecognizedTruthTier(t *testing.T) {
	factsJSON, err := json.Marshal([]map[string]any{
		{
			"subject": "good", "predicate": "p", "object": "o",
			"truth_tier": "axiomatic", "category": "clinical", "topics": []string{}, "note": "",
		},
		{
			"subject":    "bad",
			"predicate":  "p",
			"object":     "o",
			"truth_tier": "not-a-real-tier",
			"category":   "clinical",
			"topics":     []string{},
			"note":       "",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake facts: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, string(factsJSON)))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	facts, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 1 || facts[0].Subject != "good" {
		t.Errorf("expected only the fact with a recognized truth_tier to survive, got %+v", facts)
	}
}

func TestExtractionClient_Extract_SkipsUnrecognizedCategory(t *testing.T) {
	factsJSON, err := json.Marshal([]map[string]any{
		{
			"subject": "good", "predicate": "p", "object": "o",
			"truth_tier": "axiomatic", "category": "clinical", "topics": []string{}, "note": "",
		},
		{
			"subject":    "bad",
			"predicate":  "p",
			"object":     "o",
			"truth_tier": "axiomatic",
			"category":   "not-a-real-category",
			"topics":     []string{},
			"note":       "",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal fake facts: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, string(factsJSON)))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	facts, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 1 || facts[0].Subject != "good" {
		t.Errorf("expected only the fact with a recognized category to survive, got %+v", facts)
	}
}

func TestExtractionClient_Extract_PromptIncludesPublisherContextWhenSet(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, "[]"))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	_, err := client.Extract(t.Context(), &data.ExtractionRequest{
		Text:            "text",
		DocumentTitle:   "Guideline",
		PublisherName:   "World Health Organization (WHO)",
		PublicationYear: 2023,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages, _ := gotBody["messages"].([]any)
	message, _ := messages[0].(map[string]any)
	content, _ := message["content"].(string)
	if !strings.Contains(content, "World Health Organization (WHO)") ||
		!strings.Contains(content, "2023") {
		t.Errorf("expected prompt to include publisher and year, got: %s", content)
	}
}

func TestExtractionClient_Extract_PromptOmitsPublisherContextWhenUnset(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, "[]"))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	_, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages, _ := gotBody["messages"].([]any)
	message, _ := messages[0].(map[string]any)
	content, _ := message["content"].(string)
	if strings.Contains(content, "Publisher:") {
		t.Errorf("expected no Publisher line when unset, got: %s", content)
	}
}

func TestExtractionClient_Extract_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	if _, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"}); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestExtractionClient_Extract_TruncatedByMaxTokens_ReturnsClearError(t *testing.T) {
	// Regression check: a real chunk (a long personnel/affiliation roster)
	// produced enough facts to get cut off mid-fact at the previous
	// 1500-token budget, surfacing as a cryptic "unexpected end of JSON
	// input" every retry. finish_reason=length must now be caught explicitly
	// instead of falling through to a JSON-unmarshal error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `[{"subject": "Alice", "predicate":`,
					},
					"finish_reason": "length",
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal fake truncated response: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	_, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"})
	if err == nil {
		t.Fatal("expected an error for a truncated response")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("expected error to mention truncation, got: %v", err)
	}
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Errorf("expected a clear truncation error, not a raw JSON-unmarshal error: %v", err)
	}
}

func TestExtractionClient_Extract_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(chatResponseBody(t, "not valid json at all"))
	}))
	defer server.Close()

	client := external.NewExtractionClient(server.URL, "some-model", "")
	if _, err := client.Extract(t.Context(), &data.ExtractionRequest{Text: "text"}); err == nil {
		t.Fatal("expected an error for a malformed JSON response")
	}
}
