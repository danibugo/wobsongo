package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// DocumentRepo is a Postgres-backed implementation of data.DocumentRepoer.
type DocumentRepo struct {
	q           *db.Queries
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
	tx          pgx.Tx // set only on the tx-scoped instance WithTx constructs; nil otherwise
}

// Ensure DocumentRepo implements data.DocumentRepoer.
var _ data.DocumentRepoer = (*DocumentRepo)(nil)

// NewDocumentRepo creates a new Postgres-backed document repository.
// q is accepted externally (not built internally from pool) so callers
// (including tests) can supply a tx-scoped *db.Queries.
func NewDocumentRepo(
	q *db.Queries,
	pool *pgxpool.Pool,
	riverClient *river.Client[pgx.Tx],
) data.DocumentRepoer {
	return &DocumentRepo{q: q, pool: pool, riverClient: riverClient}
}

// GetByID retrieves a document by its ID.
func (r *DocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Document, error) {
	doc, err := r.q.GetDocumentByID(ctx, id)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return toModelDocument(&doc), nil
}

// GetBySHA256 retrieves a document by its content hash. Returns
// data.ErrNotFound if no document has that hash.
func (r *DocumentRepo) GetBySHA256(ctx context.Context, sha256 string) (*model.Document, error) {
	doc, err := r.q.GetDocumentBySHA256(ctx, sha256)
	if err != nil {
		return nil, mapPostgresError(err)
	}
	return toModelDocument(&doc), nil
}

// Create inserts a new document.
func (r *DocumentRepo) Create(ctx context.Context, entity *model.Document) error {
	created, err := r.q.CreateDocument(ctx, db.CreateDocumentParams{
		ID:              entity.ID,
		CreatedAt:       entity.CreatedAt,
		ModifiedAt:      entity.ModifiedAt,
		IngestedAt:      toPgTimestamptz(entity.IngestedAt),
		FileKey:         string(entity.FileURL),
		Sha256:          entity.SHA256,
		Title:           entity.Title,
		Filename:        entity.Filename,
		Filetype:        entity.Filetype,
		Filesize:        entity.Filesize,
		PageCount:       toInt32(entity.PageCount),
		PublisherName:   entity.PublisherName,
		PublicationYear: toInt32(entity.PublicationYear),
		Language:        toInt32(int(entity.Language)),
	})
	if err != nil {
		return mapPostgresError(err)
	}
	*entity = *toModelDocument(&created)
	return nil
}

// Update saves an existing document.
func (r *DocumentRepo) Update(ctx context.Context, entity *model.Document) error {
	updated, err := r.q.UpdateDocument(ctx, db.UpdateDocumentParams{
		ID:              entity.ID,
		ModifiedAt:      entity.ModifiedAt,
		IngestedAt:      toPgTimestamptz(entity.IngestedAt),
		FileKey:         string(entity.FileURL),
		Sha256:          entity.SHA256,
		Title:           entity.Title,
		Filename:        entity.Filename,
		Filetype:        entity.Filetype,
		Filesize:        entity.Filesize,
		PageCount:       toInt32(entity.PageCount),
		PublisherName:   entity.PublisherName,
		PublicationYear: toInt32(entity.PublicationYear),
		Language:        toInt32(int(entity.Language)),
	})
	if err != nil {
		return mapPostgresError(err)
	}
	*entity = *toModelDocument(&updated)
	return nil
}

// Delete removes a document by its ID.
func (r *DocumentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteDocument(ctx, id); err != nil {
		return mapPostgresError(err)
	}
	return nil
}

// Paginate retrieves a paginated list of documents.
func (r *DocumentRepo) Paginate(
	ctx context.Context,
	q data.SupportsPagination,
) (*dto.PaginationResults[model.Document], error) {
	limit, offset := q.Limit(), q.Offset()

	rows, err := r.q.PaginateDocuments(
		ctx,
		db.PaginateDocumentsParams{Limit: limit, Offset: offset},
	)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	total, err := r.q.CountDocuments(ctx)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	items := make([]model.Document, 0, len(rows))
	for i := range rows {
		items = append(items, *toModelDocument(&rows[i]))
	}

	page := int32(1)
	if limit > 0 {
		page = offset/limit + 1
	}

	return &dto.PaginationResults[model.Document]{
		Page:       int(page),
		PerPage:    int(limit),
		TotalItems: int(total),
		Items:      items,
	}, nil
}

// WithTx executes fn within a Postgres transaction, giving it a
// transaction-scoped repo whose Enqueue calls are part of the same
// transaction as any CRUD calls it makes.
func (r *DocumentRepo) WithTx(ctx context.Context, fn func(data.DocumentRepoer) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("document repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &DocumentRepo{
		q:           r.q.WithTx(tx),
		pool:        r.pool,
		riverClient: r.riverClient,
		tx:          tx,
	}

	if err := fn(txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Enqueue adds a job to the queue, using the open transaction when called
// from within WithTx so the job insert is atomic with any CRUD writes.
func (r *DocumentRepo) Enqueue(ctx context.Context, payload queue.BackgroundJob) error {
	if r.tx != nil {
		_, err := r.riverClient.InsertTx(ctx, r.tx, payload, nil)
		return err
	}
	_, err := r.riverClient.Insert(ctx, payload, nil)
	return err
}

// toInt32 converts a small, application-bounded int (a page count or
// publication year) to int32 for sqlc-generated params. These values never
// approach int32's range in practice.
func toInt32(v int) int32 {
	return int32(v) //nolint:gosec // bounded application values (page counts, years)
}

// toModelDocument maps a sqlc-generated db.Document row to model.Document.
func toModelDocument(d *db.Document) *model.Document {
	return &model.Document{
		ID:              d.ID,
		CreatedAt:       d.CreatedAt,
		ModifiedAt:      d.ModifiedAt,
		IngestedAt:      fromPgTimestamptz(d.IngestedAt),
		FileURL:         model.S3Key(d.FileKey),
		SHA256:          d.Sha256,
		Title:           d.Title,
		Filename:        d.Filename,
		Filetype:        d.Filetype,
		Filesize:        d.Filesize,
		PageCount:       int(d.PageCount),
		PublisherName:   d.PublisherName,
		PublicationYear: int(d.PublicationYear),
		Language:        model.Language(d.Language),
	}
}

// toPgTimestamptz converts a nullable *time.Time into pgtype.Timestamptz.
func toPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// fromPgTimestamptz converts a pgtype.Timestamptz into a nullable *time.Time.
func fromPgTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
