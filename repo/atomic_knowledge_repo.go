package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/pgvector/pgvector-go"
)

// AtomicKnowledgeRepo is a Postgres-backed implementation of data.AtomicKnowledgeRepoer.
type AtomicKnowledgeRepo struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// Ensure AtomicKnowledgeRepo implements data.AtomicKnowledgeRepoer.
var _ data.AtomicKnowledgeRepoer = (*AtomicKnowledgeRepo)(nil)

// NewAtomicKnowledgeRepo creates a new Postgres-backed atomic knowledge repository.
// q is accepted externally (not built internally from pool) so callers
// (including tests) can supply a tx-scoped *db.Queries.
func NewAtomicKnowledgeRepo(q *db.Queries, pool *pgxpool.Pool) data.AtomicKnowledgeRepoer {
	return &AtomicKnowledgeRepo{q: q, pool: pool}
}

// GetByID retrieves a single fact by its ID.
func (r *AtomicKnowledgeRepo) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.AtomicKnowledge, error) {
	fact, err := r.q.GetAtomicKnowledgeByID(ctx, id)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return toModelAtomicKnowledge(&fact), nil
}

// CreateBatch inserts multiple fully-formed knowledge facts in a single COPY operation.
func (r *AtomicKnowledgeRepo) CreateBatch(
	ctx context.Context,
	knowledge []model.AtomicKnowledge,
) error {
	if len(knowledge) == 0 {
		return nil
	}

	params := make([]db.CreateAtomicKnowledgeBatchParams, len(knowledge))
	for i := range knowledge {
		params[i] = toCreateAtomicKnowledgeBatchParams(&knowledge[i])
	}

	if _, err := r.q.CreateAtomicKnowledgeBatch(ctx, params); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// MarkChunkKnowledgeExtracted records that extraction has run for the given
// chunk, even if it produced zero facts. Operates on document_chunks — see
// data.AtomicKnowledgeRepoer's doc comment for why this repo owns it rather
// than depending on DocumentChunkRepoer: sqlc generates every query onto the
// same shared *db.Queries regardless of which .sql file defines it.
func (r *AtomicKnowledgeRepo) MarkChunkKnowledgeExtracted(
	ctx context.Context,
	chunkID uuid.UUID,
) error {
	if err := r.q.MarkChunkKnowledgeExtracted(ctx, chunkID); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// ListNeedingEmbedding retrieves facts for a document that don't have an
// embedding yet, ordered by CreatedAt.
func (r *AtomicKnowledgeRepo) ListNeedingEmbedding(
	ctx context.Context,
	documentID uuid.UUID,
) ([]model.AtomicKnowledge, error) {
	rows, err := r.q.ListKnowledgeNeedingEmbedding(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	facts := make([]model.AtomicKnowledge, 0, len(rows))
	for i := range rows {
		facts = append(facts, *toModelAtomicKnowledge(&rows[i]))
	}
	return facts, nil
}

// PaginateByDocumentID retrieves a paginated page of facts for a single
// document, ordered most-recent first.
func (r *AtomicKnowledgeRepo) PaginateByDocumentID(
	ctx context.Context,
	documentID uuid.UUID,
	q data.SupportsPagination,
) (*dto.PaginationResults[model.AtomicKnowledge], error) {
	limit, offset := q.Limit(), q.Offset()

	rows, err := r.q.PaginateAtomicKnowledgeByDocumentID(
		ctx,
		db.PaginateAtomicKnowledgeByDocumentIDParams{
			DocumentID: documentID,
			Limit:      limit,
			Offset:     offset,
		},
	)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	total, err := r.q.CountAtomicKnowledgeByDocumentID(ctx, documentID)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	facts := make([]model.AtomicKnowledge, 0, len(rows))
	for i := range rows {
		facts = append(facts, *toModelAtomicKnowledge(&rows[i]))
	}

	page := int32(1)
	if limit > 0 {
		page = offset/limit + 1
	}

	return &dto.PaginationResults[model.AtomicKnowledge]{
		Page:       int(page),
		PerPage:    int(limit),
		TotalItems: int(total),
		Items:      facts,
	}, nil
}

// UpdateEmbedding persists the embedding vector for a single fact.
func (r *AtomicKnowledgeRepo) UpdateEmbedding(
	ctx context.Context,
	id uuid.UUID,
	embedding []float32,
) error {
	if err := r.q.UpdateAtomicKnowledgeEmbedding(ctx, db.UpdateAtomicKnowledgeEmbeddingParams{
		ID:        id,
		Embedding: toPgvector(embedding),
		UpdatedAt: time.Now(),
	}); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// SearchByEmbedding returns the limit facts (excluding any marked invalid or
// irrelevant, or category=metadata) whose embedding is closest (cosine
// distance) to queryVector, ordered nearest-first.
func (r *AtomicKnowledgeRepo) SearchByEmbedding(
	ctx context.Context,
	queryVector []float32,
	limit int,
) ([]data.ScoredResult[model.AtomicKnowledge], error) {
	return searchScored(
		ctx,
		r.pool,
		`SELECT id, embedding <=> $1 AS score FROM atomic_knowledge
		 WHERE embedding IS NOT NULL AND NOT marked_as_invalid AND NOT marked_as_irrelevant
		   AND category != $3
		 ORDER BY score ASC
		 LIMIT $2`,
		[]any{pgvector.NewVector(queryVector), limit, int(model.FactCategoryMetadata)},
		r.GetByID,
	)
}

// SearchByFullText returns the limit facts (excluding any marked invalid or
// irrelevant, or category=metadata) whose subject/predicate/object/note best
// match query via Postgres full-text search (ts_rank_cd), ordered
// best-first. Matches against both the English and French tsvector columns
// and takes the best of the two — see document_chunk_repo.go's
// SearchByFullText for why.
func (r *AtomicKnowledgeRepo) SearchByFullText(
	ctx context.Context,
	query string,
	limit int,
) ([]data.ScoredResult[model.AtomicKnowledge], error) {
	return searchScored(
		ctx,
		r.pool,
		`SELECT id, GREATEST(
		     ts_rank_cd(fts_en, websearch_to_tsquery('english', $1)),
		     ts_rank_cd(fts_fr, websearch_to_tsquery('french', $1))
		 ) AS score
		 FROM atomic_knowledge
		 WHERE NOT marked_as_invalid AND NOT marked_as_irrelevant
		   AND category != $3
		   AND (fts_en @@ websearch_to_tsquery('english', $1)
		        OR fts_fr @@ websearch_to_tsquery('french', $1))
		 ORDER BY score DESC
		 LIMIT $2`,
		[]any{query, limit, int(model.FactCategoryMetadata)},
		r.GetByID,
	)
}

// SearchBySimilarity returns the limit facts (excluding any marked invalid or
// irrelevant, or category=metadata) whose subject/predicate/object
// trigram-match query, ranked by the best of the three fields' similarity,
// ordered best-first.
func (r *AtomicKnowledgeRepo) SearchBySimilarity(
	ctx context.Context,
	query string,
	limit int,
) ([]data.ScoredResult[model.AtomicKnowledge], error) {
	return searchScored(
		ctx,
		r.pool,
		`SELECT id, GREATEST(similarity(subject, $1), similarity(predicate, $1), similarity(object, $1)) AS score
		 FROM atomic_knowledge
		 WHERE NOT marked_as_invalid AND NOT marked_as_irrelevant
		   AND category != $3
		   AND (subject % $1 OR predicate % $1 OR object % $1)
		 ORDER BY score DESC
		 LIMIT $2`,
		[]any{query, limit, int(model.FactCategoryMetadata)},
		r.GetByID,
	)
}

// WithTx executes fn within a Postgres transaction, giving it a
// transaction-scoped repo so CreateBatch and MarkChunkKnowledgeExtracted
// commit atomically.
func (r *AtomicKnowledgeRepo) WithTx(
	ctx context.Context,
	fn func(data.AtomicKnowledgeRepoer) error,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("atomic knowledge repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &AtomicKnowledgeRepo{
		q:    r.q.WithTx(tx),
		pool: r.pool,
	}

	if err := fn(txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// toModelAtomicKnowledge maps a sqlc-generated db.AtomicKnowledge row to model.AtomicKnowledge.
func toModelAtomicKnowledge(k *db.AtomicKnowledge) *model.AtomicKnowledge {
	return &model.AtomicKnowledge{
		ID:                   k.ID,
		CreatedAt:            k.CreatedAt,
		UpdatedAt:            k.UpdatedAt,
		DocumentID:           k.DocumentID,
		DocumentChunkID:      k.DocumentChunkID,
		TruthTier:            model.TruthTier(k.TruthTier),
		Category:             model.FactCategory(k.Category),
		Topics:               k.Topics,
		Subject:              k.Subject,
		Predicate:            k.Predicate,
		Object:               k.Object,
		Note:                 k.Note,
		Embedding:            fromPgvector(k.Embedding),
		MarkedAsInvalid:      k.MarkedAsInvalid,
		MarkedAsIrrelevant:   k.MarkedAsIrrelevant,
		Language:             model.Language(k.Language),
		SearchTextTranslated: fromPgText(k.SearchTextTranslated),
	}
}

// toCreateAtomicKnowledgeBatchParams maps a model.AtomicKnowledge to sqlc's batch-insert params.
func toCreateAtomicKnowledgeBatchParams(
	k *model.AtomicKnowledge,
) db.CreateAtomicKnowledgeBatchParams {
	return db.CreateAtomicKnowledgeBatchParams{
		ID:                   k.ID,
		CreatedAt:            k.CreatedAt,
		UpdatedAt:            k.UpdatedAt,
		DocumentID:           k.DocumentID,
		DocumentChunkID:      k.DocumentChunkID,
		TruthTier:            toInt32(int(k.TruthTier)),
		Category:             toInt32(int(k.Category)),
		Topics:               k.Topics,
		Subject:              k.Subject,
		Predicate:            k.Predicate,
		Object:               k.Object,
		Note:                 k.Note,
		MarkedAsInvalid:      k.MarkedAsInvalid,
		MarkedAsIrrelevant:   k.MarkedAsIrrelevant,
		Language:             toInt32(int(k.Language)),
		SearchTextTranslated: toPgText(k.SearchTextTranslated),
	}
}
