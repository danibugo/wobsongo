package data

import "context"

// CaptionRequest bundles an image with enough surrounding document context
// for a VLM to produce a grounded, detailed, searchable description.
type CaptionRequest struct {
	// ImageBytes is the raw, decoded image content.
	ImageBytes []byte

	// ContentType is the image's MIME type, e.g. "image/png".
	ContentType string

	// DocumentTitle is the parent document's title, for grounding.
	DocumentTitle string

	// Page is the 1-indexed page the image appears on.
	Page int

	// SurroundingText is nearby body text/captions gathered from the same
	// document, for grounding. May be empty if none was found.
	SurroundingText string
}

// ImageCaptioner generates a textual description of an image, so it can be
// stored as a document chunk's Text and be embeddable/searchable like any
// other chunk. Provider-agnostic by design; see external.VLMClient for the
// concrete implementation (a generic OpenAI-compatible vision chat API).
type ImageCaptioner interface {
	// Caption returns a detailed description of req's image.
	Caption(ctx context.Context, req *CaptionRequest) (string, error)
}
