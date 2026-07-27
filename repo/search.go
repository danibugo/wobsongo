package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/jackc/pgx/v5/pgxpool"
)

// searchScored runs a raw (id, score) query — ordered/limited by the
// caller's SQL, not by this helper — and hydrates each row via getByID,
// preserving the query's order. Shared by every SearchBy* method across
// DocumentChunkRepo/AtomicKnowledgeRepo: same shape (raw scan → hydrate →
// wrap), different SQL/hydrator per call site. Bypasses db.Queries/sqlc
// entirely — none of the operators these queries use (pgvector's <=>,
// full-text's @@/ts_rank_cd, pg_trgm's %/similarity()) have a precedent of
// being sqlc-parsed in this repo, and sqlc.yaml has no live DB connection to
// verify it against.
func searchScored[T any](
	ctx context.Context,
	pool *pgxpool.Pool,
	query string,
	args []any,
	getByID func(context.Context, uuid.UUID) (*T, error),
) ([]data.ScoredResult[T], error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	type idScore struct {
		id    uuid.UUID
		score float64
	}
	var pairs []idScore
	for rows.Next() {
		var p idScore
		if err := rows.Scan(&p.id, &p.score); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan search result row: %w", err)
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, mapPostgresError(err)
	}

	results := make([]data.ScoredResult[T], 0, len(pairs))
	for _, p := range pairs {
		item, err := getByID(ctx, p.id)
		if err != nil {
			return nil, fmt.Errorf("failed to hydrate search hit %s: %w", p.id, err)
		}
		results = append(results, data.ScoredResult[T]{Item: *item, Score: p.score})
	}
	return results, nil
}
