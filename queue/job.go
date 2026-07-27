// Package queue defines interfaces and types for background job processing.
package queue

type BackgroundJobType string

const (
	// JobTypeParseDocument represents a job type for parsing an ingested document via Docling.
	JobTypeParseDocument BackgroundJobType = "parse_document"

	// JobTypeProcessParsedDocument represents a job type for turning a
	// document's raw Docling output (stored in S3 by ParseDocumentWorker)
	// into stored chunks.
	JobTypeProcessParsedDocument BackgroundJobType = "process_parsed_document"

	// JobTypeCaptionImageChunks represents a job type for generating and
	// storing captions for image/chart chunks extracted during processing.
	JobTypeCaptionImageChunks BackgroundJobType = "caption_image_chunks"

	// JobTypeEmbedChunks represents a job type for computing and storing
	// embeddings for a document's text-bearing, not-yet-embedded chunks.
	JobTypeEmbedChunks BackgroundJobType = "embed_chunks"

	// JobTypeExtractKnowledge represents a job type for extracting atomic
	// knowledge facts from a document's text-bearing, not-yet-extracted chunks.
	JobTypeExtractKnowledge BackgroundJobType = "extract_knowledge"

	// JobTypeEmbedKnowledge represents a job type for computing and storing
	// embeddings for a document's not-yet-embedded atomic knowledge facts.
	JobTypeEmbedKnowledge BackgroundJobType = "embed_knowledge"

	// JobTypeTranslateChunks represents a job type for translating a
	// document's text-bearing, not-yet-translated chunks into the other
	// supported language, for cross-lingual full-text search.
	JobTypeTranslateChunks BackgroundJobType = "translate_chunks"

	// JobTypeClaimCheck represents a job type for running the claim-checking.
	JobTypeClaimCheck BackgroundJobType = "claim_check"
)

// BackgroundJob represents a generic background job.
// It is strictly compatible with River's job system.
type BackgroundJob interface {
	// Kind returns the kind of the job.
	Kind() string
}

// Queue names — each job DTO declares its queue via InsertOpts()
// (river.JobArgsWithInsertOpts), so callers enqueueing via queue.JobEnqueuer
// never need to specify it themselves — see cmd/server.go for where these are
// registered with MaxWorkers.
const (
	// QueueDocumentIngestion is used by every job in the document-ingestion
	// pipeline: ParseDocumentDTO, ProcessParsedDocumentDTO,
	// CaptionImageChunksDTO, EmbedChunksDTO, ExtractKnowledgeDTO,
	// EmbedKnowledgeDTO, TranslateChunksDTO.
	QueueDocumentIngestion = "document_ingestion"
)
