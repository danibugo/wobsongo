package repo

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kairosedubf/wobsongo/data"
)

// pgUniqueViolation is the Postgres error code for unique_violation.
// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgUniqueViolation = "23505"

// mapPostgresError translates a low-level pgx/PostgreSQL error into one of
// the application-level sentinel errors in internal/data, per ADR 0001
// ("repos map database errors to application-level errors"). Repos should
// never let raw pgx/pgconn errors escape to the service layer.
func mapPostgresError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return data.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return data.ErrConflict
	}

	return fmt.Errorf("%w: %w", data.ErrInternal, err)
}
