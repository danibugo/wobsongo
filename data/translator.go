package data

import (
	"context"

	"github.com/kairosedubf/wobsongo/model"
)

// Translator translates text from a known source language into the other
// supported language (English<->French). Provider-agnostic by design, same
// as KnowledgeExtractor/Embedder; see external.TranslationClient for the
// concrete implementation (a generic OpenAI-compatible text
// chat-completions API).
type Translator interface {
	// Translate returns text translated out of sourceLanguage into whichever
	// of the two supported languages sourceLanguage isn't.
	Translate(ctx context.Context, text string, sourceLanguage model.Language) (string, error)
}
