package repo_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// testEmbedding builds a document_chunks.embedding-dimension (1024) vector
// whose values are derived from seed, so distinct test vectors are easy to
// tell apart in failure output without hardcoding 1024 literals per test.
func testEmbedding(seed float32) []float32 {
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = seed
	}
	return vec
}

func newTestDocumentChunk(documentID uuid.UUID, seq int) model.DocumentChunk {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return model.DocumentChunk{
		ID:              uuid.New(),
		CreatedAt:       now,
		UpdatedAt:       now,
		DocumentID:      documentID,
		SequenceNumber:  seq,
		Topics:          []string{"topic-a"},
		FactualityScore: 0.5,
		ParsedChunk: model.ParsedChunk{
			Text:        "chunk text",
			Page:        1,
			LayoutType:  model.LayoutTypeParagraph,
			BoundingBox: model.BoundingBox{1, 2, 3, 4},
		},
	}
}

func TestDocumentChunkRepo_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, _ := testhelpers.SetupTestDB(t)
	defer pool.Close()

	t.Run("CreateBatch_And_ListByDocumentID", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)
			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error creating parent document: %v", err)
			}

			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			chunks := []model.DocumentChunk{
				newTestDocumentChunk(doc.ID, 0),
				newTestDocumentChunk(doc.ID, 1),
			}
			if err := chunkRepo.CreateBatch(ctx, chunks); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := chunkRepo.ListByDocumentID(ctx, doc.ID)
			if err != nil {
				t.Fatalf("unexpected error listing chunks: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("expected 2 chunks, got %d", len(got))
			}
			if got[0].SequenceNumber != 0 || got[1].SequenceNumber != 1 {
				t.Errorf("expected chunks ordered by sequence number, got %+v", got)
			}
		})
	})

	t.Run("CreateBatch_Empty_NoOp", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			if err := chunkRepo.CreateBatch(ctx, nil); err != nil {
				t.Errorf("expected no error for an empty batch, got %v", err)
			}
		})
	})

	t.Run("GetByID_Success", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)
			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			chunk := newTestDocumentChunk(doc.ID, 0)
			if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := chunkRepo.GetByID(ctx, chunk.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Text != chunk.Text || got.BoundingBox != chunk.BoundingBox {
				t.Errorf("fetched chunk does not match created one: %+v vs %+v", got, chunk)
			}
		})
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			_, err := chunkRepo.GetByID(ctx, uuid.New())
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

			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			chunk := newTestDocumentChunk(doc.ID, 0)
			if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			chunk.Text = "updated text"
			chunk.Topics = []string{"updated-topic"}
			chunk.FactualityScore = 0.9
			if err := chunkRepo.Update(ctx, &chunk); err != nil {
				t.Fatalf("unexpected error updating chunk: %v", err)
			}

			got, err := chunkRepo.GetByID(ctx, chunk.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Text != "updated text" || got.FactualityScore != 0.9 {
				t.Errorf("update did not persist: %+v", got)
			}
		})
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			chunk := newTestDocumentChunk(uuid.New(), 0)
			err := chunkRepo.Update(ctx, &chunk)
			if !errors.Is(err, data.ErrNotFound) {
				t.Errorf("expected data.ErrNotFound, got %v", err)
			}
		})
	})

	t.Run("Update_PersistsAndRoundTripsEmbedding", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)
			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			chunk := newTestDocumentChunk(doc.ID, 0)
			if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := chunkRepo.GetByID(ctx, chunk.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Embedding != nil {
				t.Errorf(
					"expected a freshly created chunk to have a nil embedding, got %v",
					got.Embedding,
				)
			}

			want := testEmbedding(0.1)
			got.Embedding = want
			if err := chunkRepo.Update(ctx, got); err != nil {
				t.Fatalf("unexpected error updating embedding: %v", err)
			}

			roundTripped, err := chunkRepo.GetByID(ctx, chunk.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(roundTripped.Embedding) != len(want) {
				t.Fatalf("expected embedding %v, got %v", want, roundTripped.Embedding)
			}
			for i := range want {
				if roundTripped.Embedding[i] != want[i] {
					t.Errorf("expected embedding %v, got %v", want, roundTripped.Embedding)
					break
				}
			}
		})
	})

	t.Run("ListChunksNeedingEmbedding_FiltersToUnembeddedTextChunks", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			documentRepo := repo.NewDocumentRepo(q, pool, nil)
			doc := newTestDocument(uuid.NewString())
			if err := documentRepo.Create(ctx, doc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)

			needsEmbedding := newTestDocumentChunk(doc.ID, 0)
			alreadyEmbedded := newTestDocumentChunk(doc.ID, 1)
			blankText := newTestDocumentChunk(doc.ID, 2)
			blankText.Text = ""
			if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{
				needsEmbedding, alreadyEmbedded, blankText,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			alreadyEmbedded.Embedding = testEmbedding(0.4)
			if err := chunkRepo.Update(ctx, &alreadyEmbedded); err != nil {
				t.Fatalf("unexpected error embedding chunk: %v", err)
			}

			got, err := chunkRepo.ListChunksNeedingEmbedding(ctx, doc.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("expected exactly 1 chunk needing embedding, got %d: %+v", len(got), got)
			}
			if got[0].ID != needsEmbedding.ID {
				t.Errorf(
					"expected the unembedded text chunk %s, got %s",
					needsEmbedding.ID, got[0].ID,
				)
			}
		})
	})

	t.Run(
		"ListChunksNeedingKnowledgeExtraction_FiltersToUnextractedTextChunks",
		func(t *testing.T) {
			testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
				documentRepo := repo.NewDocumentRepo(q, pool, nil)
				doc := newTestDocument(uuid.NewString())
				if err := documentRepo.Create(ctx, doc); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
				knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)

				needsExtraction := newTestDocumentChunk(doc.ID, 0)
				alreadyExtracted := newTestDocumentChunk(doc.ID, 1)
				blankText := newTestDocumentChunk(doc.ID, 2)
				blankText.Text = ""
				if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{
					needsExtraction, alreadyExtracted, blankText,
				}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if err := knowledgeRepo.MarkChunkKnowledgeExtracted(
					ctx,
					alreadyExtracted.ID,
				); err != nil {
					t.Fatalf("unexpected error marking chunk extracted: %v", err)
				}

				got, err := chunkRepo.ListChunksNeedingKnowledgeExtraction(ctx, doc.ID)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != 1 {
					t.Fatalf(
						"expected exactly 1 chunk needing knowledge extraction, got %d: %+v",
						len(got), got,
					)
				}
				if got[0].ID != needsExtraction.ID {
					t.Errorf(
						"expected the unextracted text chunk %s, got %s",
						needsExtraction.ID, got[0].ID,
					)
				}
			})
		},
	)

	t.Run("ShouldBeStored_ExcludesReferenceLayout", func(t *testing.T) {
		testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
			chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
			doc := model.Document{ID: uuid.New()}

			paragraph := newTestDocumentChunk(uuid.New(), 0)
			ok, err := chunkRepo.ShouldBeStored(ctx, doc, paragraph)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Error("expected a paragraph-layout chunk to be stored")
			}

			reference := newTestDocumentChunk(uuid.New(), 0)
			reference.LayoutType = model.LayoutTypeReference
			ok, err = chunkRepo.ShouldBeStored(ctx, doc, reference)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Error("expected a reference-layout chunk not to be stored")
			}
		})
	})
}

// TestDocumentChunkRepo_WithTx_Enqueue exercises real transactional
// atomicity, so it cannot use testhelpers.WithTxRollback — see the identical
// reasoning in TestDocumentRepo_WithTx_Enqueue.
func TestDocumentChunkRepo_WithTx_Enqueue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	// t.Cleanup (not defer): cleanups run in LIFO order after the test body
	// completes, so registering the pool close first and the row delete
	// second guarantees the delete runs while the pool is still open — a
	// plain `defer pool.Close()` here would fire before this delete cleanup
	// and silently no-op it (a real bug caught while writing this test).
	t.Cleanup(func() { pool.Close() })

	ctx := t.Context()

	documentRepo := repo.NewDocumentRepo(q, pool, nil)
	doc := newTestDocument(uuid.NewString())
	if err := documentRepo.Create(ctx, doc); err != nil {
		t.Fatalf("unexpected error creating parent document: %v", err)
	}
	t.Cleanup(func() {
		// ON DELETE CASCADE takes any committed chunks with it too.
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	workers := river.NewWorkers()
	river.AddWorker(workers, &noopParseDocumentWorker{})

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		t.Fatalf("failed to construct river client: %v", err)
	}

	chunkRepo := repo.NewDocumentChunkRepo(
		q,
		pool,
		func() *river.Client[pgx.Tx] { return riverClient },
	)

	t.Run("Success_CommitsChunkAndEnqueuesJob", func(t *testing.T) {
		chunk := newTestDocumentChunk(doc.ID, 0)

		err := chunkRepo.WithTx(ctx, func(tx data.DocumentChunkRepoer) error {
			if err := tx.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
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

		if _, err := chunkRepo.GetByID(ctx, chunk.ID); err != nil {
			t.Errorf("expected chunk to be committed, got error: %v", err)
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

	t.Run("Failure_RollsBackChunkCreate", func(t *testing.T) {
		chunk := newTestDocumentChunk(doc.ID, 1)

		err := chunkRepo.WithTx(ctx, func(tx data.DocumentChunkRepoer) error {
			if err := tx.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
				return err
			}
			return errors.New("boom")
		})
		if err == nil {
			t.Fatal("expected an error from WithTx")
		}

		if _, err := chunkRepo.GetByID(ctx, chunk.ID); !errors.Is(err, data.ErrNotFound) {
			t.Errorf("expected the chunk create to be rolled back, got: %v", err)
		}
	})
}

// testDirectionalEmbedding returns a 1024-dim one-hot vector (1.0 at index,
// 0 elsewhere). Unlike testEmbedding's uniform vectors — which are always
// parallel to each other regardless of seed, giving zero cosine distance
// between any two — one-hot vectors at different indices are orthogonal
// (cosine distance 1), so they're meaningfully distinguishable for
// SearchByEmbedding ordering tests.
func testDirectionalEmbedding(index int) []float32 {
	vec := make([]float32, 1024)
	vec[index] = 1.0
	return vec
}

// TestDocumentChunkRepo_SearchByEmbedding_OrdersByDistance is an integration
// test using real commits, not testhelpers.WithTxRollback: SearchByEmbedding
// queries via r.pool directly (raw SQL, not the tx-scoped db.Queries), so
// rows inserted inside a rolled-back transaction on a different connection
// would never be visible to it. Same reasoning as
// TestDocumentChunkRepo_WithTx_Enqueue/TestAtomicKnowledgeRepo_WithTx.
func TestDocumentChunkRepo_SearchByEmbedding_OrdersByDistance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	t.Cleanup(func() { pool.Close() })
	ctx := t.Context()

	documentRepo := repo.NewDocumentRepo(q, pool, nil)
	doc := newTestDocument(uuid.NewString())
	if err := documentRepo.Create(ctx, doc); err != nil {
		t.Fatalf("unexpected error creating parent document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
	near := newTestDocumentChunk(doc.ID, 0)
	far := newTestDocumentChunk(doc.ID, 1)
	unembedded := newTestDocumentChunk(doc.ID, 2)
	if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{near, far, unembedded}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nearVec := testDirectionalEmbedding(0)
	farVec := testDirectionalEmbedding(500)
	for id, vec := range map[uuid.UUID][]float32{near.ID: nearVec, far.ID: farVec} {
		chunk, err := chunkRepo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		chunk.Embedding = vec
		if err := chunkRepo.Update(ctx, chunk); err != nil {
			t.Fatalf("unexpected error embedding chunk: %v", err)
		}
	}
	// unembedded is left with a nil embedding — must never appear in results.

	// Query against a generous limit, not 2: this DB may carry real embedded
	// chunks from unrelated documents (this repo's dev DB does, from manual
	// pipeline testing), so the two test rows aren't guaranteed to be the
	// only — or even the top — global matches. Assert their relative order
	// and presence by ID instead of exact result count/position.
	results, err := chunkRepo.SearchByEmbedding(ctx, nearVec, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nearIdx, farIdx := -1, -1
	for i, r := range results {
		switch r.Item.ID {
		case near.ID:
			nearIdx = i
		case far.ID:
			farIdx = i
		case unembedded.ID:
			t.Errorf("expected the unembedded chunk to be excluded from results")
		}
	}
	if nearIdx == -1 {
		t.Fatalf("expected the near chunk to appear in results")
	}
	if farIdx == -1 {
		t.Fatalf("expected the far chunk to appear in results")
	}
	if nearIdx >= farIdx {
		t.Errorf(
			"expected near chunk (rank %d) to outrank far chunk (rank %d)",
			nearIdx, farIdx,
		)
	}
	if results[nearIdx].Score >= results[farIdx].Score {
		t.Errorf(
			"expected ascending distance order, got near=%v far=%v",
			results[nearIdx].Score, results[farIdx].Score,
		)
	}
}

// TestDocumentChunkRepo_SearchByFullText_OrdersByRank is a real-commit
// integration test — see TestDocumentChunkRepo_SearchByEmbedding_OrdersByDistance's
// doc comment for why WithTxRollback can't be used here.
func TestDocumentChunkRepo_SearchByFullText_OrdersByRank(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	t.Cleanup(func() { pool.Close() })
	ctx := t.Context()

	documentRepo := repo.NewDocumentRepo(q, pool, nil)
	doc := newTestDocument(uuid.NewString())
	if err := documentRepo.Create(ctx, doc); err != nil {
		t.Fatalf("unexpected error creating parent document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
	relevant := newTestDocumentChunk(doc.ID, 0)
	relevant.Text = "giraffes have unusually long necks for reaching tall trees"
	irrelevant := newTestDocumentChunk(doc.ID, 1)
	irrelevant.Text = "the quarterly financial report was filed on time"
	if err := chunkRepo.CreateBatch(
		ctx,
		[]model.DocumentChunk{relevant, irrelevant},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, err := chunkRepo.SearchByFullText(ctx, "giraffe necks", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 matching chunk, got %d", len(results))
	}
	if results[0].Item.ID != relevant.ID {
		t.Errorf("expected the giraffe chunk to match, got %s", results[0].Item.ID)
	}
}
