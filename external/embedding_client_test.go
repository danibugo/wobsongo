package external_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
)

func TestEmbeddingClient_Embed_Success(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Deliberately out of order — Embed must re-sort by index.
		_, _ = w.Write([]byte(`{"data":[
			{"embedding":[0.3,0.4],"index":1},
			{"embedding":[0.1,0.2],"index":0}
		]}`))
	}))
	defer server.Close()

	client := external.NewEmbeddingClient(server.URL, "text-embedding-3-small", "test-api-key")

	vectors, err := client.Embed(t.Context(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if vectors[0][0] != 0.1 || vectors[0][1] != 0.2 {
		t.Errorf("expected vectors[0] = [0.1 0.2] (re-sorted by index), got %v", vectors[0])
	}
	if vectors[1][0] != 0.3 || vectors[1][1] != 0.4 {
		t.Errorf("expected vectors[1] = [0.3 0.4] (re-sorted by index), got %v", vectors[1])
	}

	if gotPath != "/v1/embeddings" {
		t.Errorf("expected path /v1/embeddings, got %s", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer test-api-key", gotAuth)
	}
	if gotBody["model"] != "text-embedding-3-small" {
		t.Errorf("expected model %q, got %v", "text-embedding-3-small", gotBody["model"])
	}
	input, ok := gotBody["input"].([]any)
	if !ok || len(input) != 2 || input[0] != "first" || input[1] != "second" {
		t.Errorf("expected input [\"first\" \"second\"], got %v", gotBody["input"])
	}
}

func TestEmbeddingClient_Embed_NoAPIKey_OmitsAuthHeader(t *testing.T) {
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1],"index":0}]}`))
	}))
	defer server.Close()

	client := external.NewEmbeddingClient(server.URL, "some-model", "")
	if _, err := client.Embed(t.Context(), []string{"text"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header when apiKey is empty, got %q", gotAuth)
	}
}

func TestEmbeddingClient_Embed_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := external.NewEmbeddingClient(server.URL, "some-model", "")
	if _, err := client.Embed(t.Context(), []string{"text"}); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestEmbeddingClient_Embed_MismatchedCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1],"index":0}]}`))
	}))
	defer server.Close()

	client := external.NewEmbeddingClient(server.URL, "some-model", "")
	if _, err := client.Embed(t.Context(), []string{"first", "second"}); err == nil {
		t.Fatal("expected an error when the response vector count doesn't match the input count")
	}
}
