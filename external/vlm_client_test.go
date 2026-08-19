package external_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/external"
)

func TestVLMClient_Caption_Success(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a detailed caption"}}]}`))
	}))
	defer server.Close()

	client := external.NewVLMClient(server.URL, "qwen2-vl-7b-instruct", "test-api-key")

	caption, err := client.Caption(t.Context(), &data.CaptionRequest{
		ImageBytes:      []byte("fake-image-bytes"),
		ContentType:     "image/png",
		DocumentTitle:   "My Document",
		Page:            3,
		SurroundingText: "some nearby text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caption != "a detailed caption" {
		t.Errorf("expected caption %q, got %q", "a detailed caption", caption)
	}

	if gotPath != "/v1/chat/completions" {
		t.Errorf("expected path /v1/chat/completions, got %s", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer test-api-key", gotAuth)
	}
	if gotBody["model"] != "qwen2-vl-7b-instruct" {
		t.Errorf("expected model %q, got %v", "qwen2-vl-7b-instruct", gotBody["model"])
	}

	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected exactly 1 message, got %v", gotBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message to be an object, got %T", messages[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("expected exactly 2 content parts, got %v", message["content"])
	}

	textPart, ok := content[0].(map[string]any)
	if !ok || textPart["type"] != "text" {
		t.Fatalf("expected first content part to be type=text, got %v", content[0])
	}
	prompt, _ := textPart["text"].(string)
	if !strings.Contains(prompt, "My Document") || !strings.Contains(prompt, "some nearby text") {
		t.Errorf("expected prompt to include document title and surrounding text, got: %s", prompt)
	}

	imagePart, ok := content[1].(map[string]any)
	if !ok || imagePart["type"] != "image_url" {
		t.Fatalf("expected second content part to be type=image_url, got %v", content[1])
	}
	imageURL, ok := imagePart["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("expected image_url to be an object, got %v", imagePart["image_url"])
	}
	wantDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(
		[]byte("fake-image-bytes"),
	)
	if imageURL["url"] != wantDataURL {
		t.Errorf("expected image data URL %q, got %v", wantDataURL, imageURL["url"])
	}
}

func TestVLMClient_Caption_NoAPIKey_OmitsAuthHeader(t *testing.T) {
	var gotAuth string
	var authHeaderSet bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, authHeaderSet = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"caption"}}]}`))
	}))
	defer server.Close()

	client := external.NewVLMClient(server.URL, "some-model", "")
	if _, err := client.Caption(
		t.Context(),
		&data.CaptionRequest{ContentType: "image/png"},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authHeaderSet {
		t.Errorf("expected no Authorization header when apiKey is empty, got %q", gotAuth)
	}
}

func TestVLMClient_Caption_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("vlm exploded"))
	}))
	defer server.Close()

	client := external.NewVLMClient(server.URL, "some-model", "")
	_, err := client.Caption(t.Context(), &data.CaptionRequest{ContentType: "image/png"})
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "vlm exploded") {
		t.Errorf("expected error to include response body, got: %v", err)
	}
}

func TestVLMClient_Caption_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	client := external.NewVLMClient(server.URL, "some-model", "")
	_, err := client.Caption(t.Context(), &data.CaptionRequest{ContentType: "image/png"})
	if err == nil {
		t.Fatal("expected an error when the response has no choices")
	}
}

func TestVLMClient_Caption_EmptyCaptionContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "}}]}`))
	}))
	defer server.Close()

	client := external.NewVLMClient(server.URL, "some-model", "")
	_, err := client.Caption(t.Context(), &data.CaptionRequest{ContentType: "image/png"})
	if err == nil {
		t.Fatal("expected an error when the caption content is blank")
	}
}
