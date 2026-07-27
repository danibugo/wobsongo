package data

import (
	"context"

	"github.com/kairosedubf/wobsongo/model"
)

// ProcessedDocument is the parsed, unfiltered output of an external document processor.
type ProcessedDocument struct {
	// Title is the document-level title, if the processor could detect one.
	Title string

	// PageCount is the total number of pages in the document.
	PageCount int

	// Chunks contains every extracted element, unfiltered. Callers decide
	// which layout types to keep.
	Chunks []model.ParsedChunk
}

// DocumentProcessor defines the contract for fetching a document's raw,
// unparsed processor output. Deliberately returns raw bytes, not structured
// chunks — the fetch (a slow, expensive external call) and the response
// interpretation are separate, independently retryable steps; see
// external.ParseRaw for the latter.
type DocumentProcessor interface {
	// FetchRawFromURL fetches the document at documentURL and returns the
	// processor's raw, unparsed response body.
	FetchRawFromURL(ctx context.Context, documentURL string) ([]byte, error)
}
