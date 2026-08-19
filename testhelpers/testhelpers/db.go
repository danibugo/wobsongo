package testhelpers

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/repo"
)

// SetupTestDB connects to the Postgres instance addressed by APP_DB_URI,
// for use by repo integration tests. Fails the test immediately if the
// env var is unset or the connection can't be established.
func SetupTestDB(t *testing.T) (*pgxpool.Pool, *db.Queries) {
	t.Helper()

	uri := os.Getenv("APP_DB_URI")
	if uri == "" {
		t.Fatal("APP_DB_URI must be set to run repo integration tests")
	}

	pool, err := repo.NewPgxPool(t.Context(), uri)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	return pool, db.New(pool)
}

// WithTxRollback runs fn inside a Postgres transaction that is always rolled
// back afterward, regardless of outcome — giving each test a clean, isolated
// view of the database without needing to truncate tables between runs.
func WithTxRollback(t *testing.T, pool *pgxpool.Pool, fn func(ctx context.Context, q *db.Queries)) {
	t.Helper()

	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin test tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	fn(ctx, db.New(tx))
}
