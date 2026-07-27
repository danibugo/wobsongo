package data

import (
	"context"

	"github.com/kairosedubf/wobsongo/model"
)

// ExtractionRequest bundles a chunk's text with enough surrounding document
// context for an LLM to extract grounded, attributable atomic facts —
// including distinguishing clinical content from the document's own
// metadata (e.g. recognizing "this is the guideline's own front matter, not
// clinical content").
type ExtractionRequest struct {
	// Text is the chunk's content to extract facts from.
	Text string

	// DocumentTitle is the parent document's title, for grounding.
	DocumentTitle string

	// PublisherName is the parent document's publisher, for grounding.
	PublisherName string

	// PublicationYear is the parent document's publication year, for grounding.
	PublicationYear int

	// Language is the source chunk's language — tells the extractor what
	// language to preserve subject/predicate/object/note in, and which
	// language to translate the search text into (the other of the two
	// supported languages).
	Language model.Language
}

// ExtractedFact is a single subject-predicate-object fact extracted from a
// chunk, before it's assigned an ID/DocumentID/DocumentChunkID and persisted
// as a model.AtomicKnowledge.
type ExtractedFact struct {
	Subject   string
	Predicate string
	Object    string
	Note      string
	TruthTier model.TruthTier
	Category  model.FactCategory
	Topics    []string

	// TranslatedSearchText is a concatenated subject+predicate+object+note
	// blob translated into the other of the two supported languages from
	// ExtractionRequest.Language — used purely for cross-lingual full-text
	// search, never shown in citations/display.
	TranslatedSearchText string
}

// KnowledgeExtractor extracts zero or more atomic facts from a chunk of
// text. Provider-agnostic by design, same as ImageCaptioner/Embedder; see
// external.ExtractionClient for the concrete implementation (a generic
// OpenAI-compatible text chat-completions API).
type KnowledgeExtractor interface {
	// Extract returns the facts found in req.Text, or an empty slice if none.
	Extract(ctx context.Context, req *ExtractionRequest) ([]ExtractedFact, error)
}
