package repo_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// noopParseDocumentWorker satisfies River's client-side check that a job's
// kind is registered in the Workers bundle before it can be inserted. This
// test never starts the client, so Work is never actually invoked.
type noopParseDocumentWorker struct {
	river.WorkerDefaults[queue.ParseDocumentDTO]
}

func (*noopParseDocumentWorker) Work(context.Context, *river.Job[queue.ParseDocumentDTO]) error {
	return nil
}

func newTestDocument(sha256 string) *model.Document {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &model.Document{
		ID:              uuid.New(),
		CreatedAt:       now,
		ModifiedAt:      now,
		FileURL:         model.S3Key("documents/" + sha256 + ".pdf"),
		SHA256:          sha256,
		Title:           "Test Document",
		Filename:        "test.pdf",
		Filetype:        "application/pdf",
		Filesize:        1024,
		PageCount:       10,
		PublisherName:   "Test Press",
		PublicationYear: 2024,
	}
}

func TestDocumentRepo_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, _ := testhelpers.SetupTestDB(t)
	defer pool.Close()

	t.Run("Create_Success", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := documentRepo.GetByID(ctx, doc.ID)
			if err != nil {
				t.Fatalf("unexpected error fetching created document: %v", err)
			}
			if got.Title != doc.Title || got.SHA256 != doc.SHA256 || got.FileURL != doc.FileURL {
				t.Errorf("fetched document does not match created one: %+v vs %+v", got, doc)
			}
		})
	})

	t.Run("Create_DuplicateSHA256_ReturnsConflict", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			sha := uuid.NewString()
			if err := documentRepo.Create(ctx, newTestDocument(sha)); err != nil {
				t.Fatalf("unexpected error on first create: %v", err)
			}

			err := documentRepo.Create(ctx, newTestDocument(sha))
			if !errors.Is(err, data.ErrConflict) {
				t.Errorf("expected data.ErrConflict, got %v", err)
			}
		})
	})

	t.Run("GetBySHA256_Success", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			sha := uuid.NewString()
			doc := newTestDocument(sha)
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := documentRepo.GetBySHA256(ctx, sha)
			if err != nil {
				t.Fatalf("unexpected error fetching by sha256: %v", err)
			}
			if got.ID != doc.ID || got.SHA256 != doc.SHA256 {
				t.Errorf("fetched document does not match created one: %+v vs %+v", got, doc)
			}
		})
	})

	t.Run("GetBySHA256_NotFound", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			_, err := documentRepo.GetBySHA256(ctx, uuid.NewString())
			if !errors.Is(err, data.ErrNotFound) {
				t.Errorf("expected data.ErrNotFound, got %v", err)
			}
		})
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			_, err := documentRepo.GetByID(ctx, uuid.New())
			if !errors.Is(err, data.ErrNotFound) {
				t.Errorf("expected data.ErrNotFound, got %v", err)
			}
		})
	})

	t.Run("Update_Success", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			doc.Title = "Updated Title"
			doc.PublisherName = "Updated Press"
			doc.PublicationYear = 2025
			if err := documentRepo.Update(ctx, doc); err != nil {
				t.Fatalf("unexpected error updating document: %v", err)
			}

			got, err := documentRepo.GetByID(ctx, doc.ID)
			if err != nil {
				t.Fatalf("unexpected error fetching updated document: %v", err)
			}
			if got.Title != "Updated Title" || got.PublisherName != "Updated Press" ||
				got.PublicationYear != 2025 {
				t.Errorf("update did not persist: %+v", got)
			}
		})
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			err := documentRepo.Update(ctx, newTestDocument(uuid.NewString()))
			if !errors.Is(err, data.ErrNotFound) {
				t.Errorf("expected data.ErrNotFound, got %v", err)
			}
		})
	})

	t.Run("Delete_Success", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := documentRepo.Delete(ctx, doc.ID); err != nil {
				t.Fatalf("unexpected error deleting document: %v", err)
			}

			_, err := documentRepo.GetByID(ctx, doc.ID)
			if !errors.Is(err, data.ErrNotFound) {
				t.Errorf("expected data.ErrNotFound after delete, got %v", err)
			}
		})
	})

	t.Run("Paginate_ReturnsItemsAndTotal", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)

			// CountDocuments is a global, unscoped count (documents aren't
			// per-tenant), so it also reflects any rows that already existed
			// before this test ran — baseline it rather than assuming the
			// table starts empty, so this test is correct against a fresh DB
			// or a populated dev DB alike.
			baseline, err := documentRepo.Paginate(ctx, &dto.PaginationDTO{Page: 1, PerPage: 1})
			if err != nil {
				t.Fatalf("unexpected error getting baseline count: %v", err)
			}

			const count = 3
			for range count {
				if err := documentRepo.Create(ctx, newTestDocument(uuid.NewString())); err != nil {
					t.Fatalf("unexpected error seeding document: %v", err)
				}
			}

			results, err := documentRepo.Paginate(ctx, &dto.PaginationDTO{Page: 1, PerPage: 2})
			if err != nil {
				t.Fatalf("unexpected error paginating: %v", err)
			}
			if results.TotalItems != baseline.TotalItems+count {
				t.Errorf(
					"expected total items %d, got %d",
					baseline.TotalItems+count,
					results.TotalItems,
				)
			}
			if len(results.Items) != 2 {
				t.Errorf("expected 2 items on page 1, got %d", len(results.Items))
			}
			if results.Page != 1 || results.PerPage != 2 {
				t.Errorf(
					"expected page=1 per_page=2, got page=%d per_page=%d",
					results.Page,
					results.PerPage,
				)
			}
		})
	})
}

// TestDocumentRepo_WithTx_Enqueue exercises real transactional atomicity, so
// it cannot use testhelpers.WithTxRollback — DocumentRepo.WithTx opens its
// own transaction via pool.Begin, independent of any outer wrapper.
func TestDocumentRepo_WithTx_Enqueue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	defer pool.Close()

	ctx := t.Context()

	// River validates at Insert-time that the job kind is registered in the
	// client's Workers bundle, even though this test never starts the client
	// to actually process jobs — it only inserts and inspects river_job
	// directly. A no-op worker satisfies that registration check.
	workers := river.NewWorkers()
	river.AddWorker(workers, &noopParseDocumentWorker{})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		t.Fatalf("failed to construct river client: %v", err)
	}

	documentRepo := repo.NewDocumentRepo(q, pool, riverClient)

	t.Run("Success_CommitsDocumentAndEnqueuesJob", func(t *testing.T) {
		doc := newTestDocument(uuid.NewString())
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
		})

		err := documentRepo.WithTx(ctx, func(tx data.DocumentRepoer) error {
			if err := tx.Create(ctx, doc); err != nil {
				return err
			}
			return tx.Enqueue(ctx, queue.ParseDocumentDTO{
				DocumentID: doc.ID,
				FileKey:    string(doc.FileURL),
			})
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := documentRepo.GetByID(ctx, doc.ID); err != nil {
			t.Errorf("expected document to be committed, got error: %v", err)
		}

		row := pool.QueryRow(
			ctx,
			"select args from river_job where kind = $1 order by id desc limit 1",
			queue.JobTypeParseDocument,
		)
		var rawArgs []byte
		if err := row.Scan(&rawArgs); err != nil {
			t.Fatalf("expected an enqueued river_job row, got error: %v", err)
		}
		var args queue.ParseDocumentDTO
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			t.Fatalf("failed to unmarshal river_job args: %v", err)
		}
		if args.DocumentID != doc.ID {
			t.Errorf("expected enqueued job for document %s, got %s", doc.ID, args.DocumentID)
		}
	})

	t.Run("Failure_RollsBackDocumentCreate", func(t *testing.T) {
		doc := newTestDocument(uuid.NewString())

		err := documentRepo.WithTx(ctx, func(tx data.DocumentRepoer) error {
			if err := tx.Create(ctx, doc); err != nil {
				return err
			}
			return errors.New("boom")
		})
		if err == nil {
			t.Fatal("expected an error from WithTx")
		}

		if _, err := documentRepo.GetByID(ctx, doc.ID); !errors.Is(err, data.ErrNotFound) {
			t.Errorf("expected the document create to be rolled back, got: %v", err)
		}
	})
}
