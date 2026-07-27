package repo

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// NewPgxPool connects a pgxpool.Pool to uri with pgvector's Go types
// (pgvector.Vector) registered on every connection the pool opens. Any
// entrypoint whose connections will scan/encode a vector-typed column (e.g.
// document_chunks.embedding) must be built through this, not pgxpool.New —
// without it, pgx doesn't know how to (de)serialize the vector wire format.
func NewPgxPool(ctx context.Context, uri string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
