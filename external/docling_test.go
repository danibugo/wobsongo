package external_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/model"
)

const cannedDoclingResponse = `{
	"status": "success",
	"document": {
		"json_content": {
			"name": "sample.pdf",
			"texts": [
				{"text": "Sample Guideline", "label": "title", "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 10, "b": 10}}]},
				{"text": "Page 1 of 5", "label": "page_footer", "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}]},
				{"text": "Some body text.", "label": "paragraph", "prov": [{"page_no": 2, "bbox": {"l": 1, "t": 2, "r": 3, "b": 4}}]}
			],
			"tables": [
				{"label": "table", "prov": [{"page_no": 3, "bbox": {"l": 0, "t": 0, "r": 5, "b": 5}}]}
			],
			"pictures": [
				{"label": "picture", "prov": [{"page_no": 4, "bbox": {"l": 0, "t": 0, "r": 2, "b": 2}}], "image": {"uri": "data:image/png;base64,aGk="}}
			]
		}
	}
}`

func TestDoclingClient_FetchRawFromURL_And_ParseRaw_Success(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/convert/source" {
			t.Errorf("expected path /v1/convert/source, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cannedDoclingResponse))
	}))
	defer server.Close()

	client := external.NewDoclingClient(server.URL)

	raw, err := client.FetchRawFromURL(context.Background(), "https://example.com/doc.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "sample.pdf") {
		t.Fatalf("expected raw response to contain the canned body, got: %s", raw)
	}

	// Regression check: on docling-serve v1.18.0, include_images/
	// include_page_images weren't independent controls — image_export_mode
	// alone decided whether picture crops or full-page renders came back, and
	// "embedded" could only ever produce the latter (confirmed against real
	// responses: every picture came back null, and "pages" carried ~193MB of
	// unread full-page renders). Fixed upstream in docling-serve v1.22.1+;
	// this deployment is now on v1.26.0, where these are correctly independent.
	options, ok := gotBody["options"].(map[string]any)
	if !ok {
		t.Fatalf("expected an options object in the request body, got: %v", gotBody)
	}
	if includeImages, _ := options["include_images"].(bool); !includeImages {
		t.Errorf("expected include_images=true in the request, got: %v", options["include_images"])
	}
	if includePageImages, ok := options["include_page_images"].(bool); !ok || includePageImages {
		t.Errorf(
			"expected include_page_images=false in the request, got: %v",
			options["include_page_images"],
		)
	}
	// Regression check: TableFormer's "accurate" mode is markedly slower than
	// "fast" and buys nothing today — doclingTableItem doesn't map table
	// cell/grid data yet, so the extra precision goes unused.
	if tableMode, _ := options["table_mode"].(string); tableMode != "fast" {
		t.Errorf("expected table_mode=fast in the request, got: %v", options["table_mode"])
	}

	result, err := external.ParseRaw(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing raw response: %v", err)
	}

	// The fixture's "title"-labeled chunk ("Sample Guideline") is the real,
	// content-derived title — doc.Name ("sample.pdf") is just the filename
	// Docling was given and must not be preferred over it.
	if result.Title != "Sample Guideline" {
		t.Errorf("expected title %q, got %q", "Sample Guideline", result.Title)
	}

	// Filtering is NOT this package's job — noise-type items (page_footer) must
	// still be present, unfiltered.
	if len(result.Chunks) != 5 {
		t.Fatalf(
			"expected 5 unfiltered chunks (3 text + 1 table + 1 picture), got %d",
			len(result.Chunks),
		)
	}

	var sawPageFooter, sawTable bool
	var picture *model.ParsedChunk
	for i, c := range result.Chunks {
		switch c.LayoutType {
		case model.LayoutTypePageFooter:
			sawPageFooter = true
		case model.LayoutTypeTable:
			sawTable = true
		case model.LayoutTypePicture:
			picture = &result.Chunks[i]
		}
	}
	if !sawPageFooter {
		t.Error("expected the noise page_footer chunk to be present, unfiltered")
	}
	if !sawTable {
		t.Error("expected the table chunk to be present")
	}
	if picture == nil {
		t.Fatal("expected the picture chunk to be present")
	}
	if string(picture.RawImageData) != "hi" {
		t.Errorf("expected decoded image bytes %q, got %q", "hi", picture.RawImageData)
	}
	if picture.RawImageContentType != "image/png" {
		t.Errorf("expected content type %q, got %q", "image/png", picture.RawImageContentType)
	}

	first := result.Chunks[0]
	if first.Text != "Sample Guideline" || first.LayoutType != model.LayoutTypeTitle ||
		first.Page != 1 {
		t.Errorf("unexpected first chunk mapping: %+v", first)
	}
}

func TestParseRaw_MalformedImageDoesNotFailParse(t *testing.T) {
	const raw = `{
		"status": "success",
		"document": {
			"json_content": {
				"name": "sample.pdf",
				"pictures": [
					{"label": "picture", "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}], "image": {"uri": "not-a-data-uri"}}
				]
			}
		}
	}`

	result, err := external.ParseRaw([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("expected the picture chunk to still be kept, got %d chunks", len(result.Chunks))
	}
	if result.Chunks[0].RawImageData != nil {
		t.Errorf(
			"expected no decoded image data for a malformed URI, got %q",
			result.Chunks[0].RawImageData,
		)
	}
}

func TestParseRaw_Title_FallsBackToSectionHeaderWhenNoTitleLabel(t *testing.T) {
	// Mirrors a real case: Docling tagged nothing "title" for this document,
	// but its actual title was the first "section_header"-labeled chunk.
	const raw = `{
		"status": "success",
		"document": {
			"json_content": {
				"name": "e146b714c0a394a860c25674f07aebcc7f84137042ff65f619e2d37484165e28.pdf",
				"texts": [
					{"text": "Guideline for the prevention, diagnosis and treatment of infertility", "label": "section_header", "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}]},
					{"text": "Some body text.", "label": "text", "prov": [{"page_no": 2, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}]}
				]
			}
		}
	}`

	result, err := external.ParseRaw([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "Guideline for the prevention, diagnosis and treatment of infertility" {
		t.Errorf(
			"expected the first section_header's text as title, got %q",
			result.Title,
		)
	}
}

func TestParseRaw_Title_FallsBackToDocNameWhenNoTitleOrSectionHeader(t *testing.T) {
	const raw = `{
		"status": "success",
		"document": {
			"json_content": {
				"name": "sample.pdf",
				"texts": [
					{"text": "Some body text.", "label": "text", "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}]}
				]
			}
		}
	}`

	result, err := external.ParseRaw([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "sample.pdf" {
		t.Errorf("expected doc.Name as a last-resort fallback, got %q", result.Title)
	}
}

func TestDoclingClient_FetchRawFromURL_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("docling exploded"))
	}))
	defer server.Close()

	client := external.NewDoclingClient(server.URL)

	_, err := client.FetchRawFromURL(context.Background(), "https://example.com/doc.pdf")
	if err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "docling exploded") {
		t.Errorf("expected error to include response body, got: %v", err)
	}
}
