package queue

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// ParseDocumentDTO is the river job kind for parsing an ingested document via Docling.
type ParseDocumentDTO struct {
	DocumentID uuid.UUID `json:"document_id"`
	FileKey    string    `json:"file_key"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (ParseDocumentDTO) Kind() string {
	return string(JobTypeParseDocument)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (ParseDocumentDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// ProcessParsedDocumentDTO is the river job kind for turning a document's
// raw Docling output (already stored in S3 by ParseDocumentWorker) into
// stored chunks.
type ProcessParsedDocumentDTO struct {
	DocumentID   uuid.UUID `json:"document_id"`
	RawOutputKey string    `json:"raw_output_key"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (ProcessParsedDocumentDTO) Kind() string {
	return string(JobTypeProcessParsedDocument)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (ProcessParsedDocumentDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// CaptionImageChunksDTO is the river job kind for generating and storing
// captions for a set of image/chart chunks belonging to one document.
type CaptionImageChunksDTO struct {
	DocumentID uuid.UUID   `json:"document_id"`
	ChunkIDs   []uuid.UUID `json:"chunk_ids"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (CaptionImageChunksDTO) Kind() string {
	return string(JobTypeCaptionImageChunks)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (CaptionImageChunksDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// EmbedChunksDTO is the river job kind for computing and storing embeddings
// for all of a document's chunks that have text but no embedding yet.
type EmbedChunksDTO struct {
	DocumentID uuid.UUID `json:"document_id"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (EmbedChunksDTO) Kind() string {
	return string(JobTypeEmbedChunks)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (EmbedChunksDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// ExtractKnowledgeDTO is the river job kind for extracting atomic knowledge
// facts from all of a document's chunks that have text but haven't had
// extraction run yet.
type ExtractKnowledgeDTO struct {
	DocumentID uuid.UUID `json:"document_id"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (ExtractKnowledgeDTO) Kind() string {
	return string(JobTypeExtractKnowledge)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (ExtractKnowledgeDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// EmbedKnowledgeDTO is the river job kind for computing and storing
// embeddings for all of a document's not-yet-embedded atomic knowledge facts.
type EmbedKnowledgeDTO struct {
	DocumentID uuid.UUID `json:"document_id"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (EmbedKnowledgeDTO) Kind() string {
	return string(JobTypeEmbedKnowledge)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (EmbedKnowledgeDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}

// TranslateChunksDTO is the river job kind for translating all of a
// document's chunks that have text but haven't been translated yet, into
// the other supported language.
type TranslateChunksDTO struct {
	DocumentID uuid.UUID `json:"document_id"`
}

// Kind implements queue.BackgroundJob and river.JobArgs.
func (TranslateChunksDTO) Kind() string {
	return string(JobTypeTranslateChunks)
}

// InsertOpts implements river.JobArgsWithInsertOpts.
func (TranslateChunksDTO) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueDocumentIngestion}
}
