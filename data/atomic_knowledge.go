package data

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
)

// AtomicKnowledgeRepoer defines the interface for interacting with atomic
// knowledge data storage. Deliberately not CrudableWithTx and doesn't embed
// queue.JobEnqueuer — knowledge facts are never listed/updated generically,
// and this repo doesn't chain to a further job (fact embedding is a separate,
// not-yet-built step).
type AtomicKnowledgeRepoer interface {
	// GetByID retrieves a single fact by its ID. Added for hybrid search
	// (service.RAGService) to hydrate search hits — no prior caller needed a
	// single-row read.
	GetByID(ctx context.Context, id uuid.UUID) (*model.AtomicKnowledge, error)

	// CreateBatch inserts multiple fully-formed knowledge facts in a single operation.
	CreateBatch(ctx context.Context, knowledge []model.AtomicKnowledge) error

	// MarkChunkKnowledgeExtracted records that extraction has run for the
	// given chunk, even if it produced zero facts.
	MarkChunkKnowledgeExtracted(ctx context.Context, chunkID uuid.UUID) error

	// ListNeedingEmbedding retrieves facts for a document that don't have an
	// embedding yet. Used by EmbedKnowledgeWorker; the filter also makes
	// retries idempotent — a fact already embedded is never returned again.
	ListNeedingEmbedding(ctx context.Context, documentID uuid.UUID) ([]model.AtomicKnowledge, error)

	// UpdateEmbedding persists the embedding vector for a single fact.
	UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error

	// SearchByEmbedding returns the limit facts (excluding any marked invalid
	// or irrelevant) whose embedding is closest (cosine distance) to
	// queryVector, ordered nearest-first. One of the hybrid-search retrieval
	// methods; see service.RAGService.
	SearchByEmbedding(
		ctx context.Context,
		queryVector []float32,
		limit int,
	) ([]ScoredResult[model.AtomicKnowledge], error)

	// SearchByFullText returns the limit facts (excluding any marked invalid
	// or irrelevant) whose subject/predicate/object/note best match query via
	// Postgres full-text search (ts_rank_cd), ordered best-first. One of the
	// hybrid-search retrieval methods; see service.RAGService.
	SearchByFullText(
		ctx context.Context,
		query string,
		limit int,
	) ([]ScoredResult[model.AtomicKnowledge], error)

	// SearchBySimilarity returns the limit facts (excluding any marked
	// invalid or irrelevant) whose subject/predicate/object trigram-match
	// query, ranked by the best of the three fields' similarity, ordered
	// best-first. One of the hybrid-search retrieval methods; see
	// service.RAGService.
	SearchBySimilarity(
		ctx context.Context,
		query string,
		limit int,
	) ([]ScoredResult[model.AtomicKnowledge], error)

	TxAware[AtomicKnowledgeRepoer]
}
