package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/pgvector/pgvector-go"
)

func newTestAtomicKnowledge(documentID, chunkID uuid.UUID) model.AtomicKnowledge {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return model.AtomicKnowledge{
		ID:              uuid.New(),
		CreatedAt:       now,
		UpdatedAt:       now,
		DocumentID:      documentID,
		DocumentChunkID: chunkID,
		TruthTier:       model.TruthTierAxiomatic,
		Topics:          []string{"topic-a"},
		Subject:         "Alice",
		Predicate:       "founded",
		Object:          "Acme Corp",
		Note:            "test fact",
	}
}

// TestAtomicKnowledgeRepo_WithTx exercises real transactional atomicity, so
// it cannot use testhelpers.WithTxRollback — AtomicKnowledgeRepo.WithTx opens
// its own transaction via pool.Begin, independent of any outer wrapper. Same
// reasoning as TestDocumentChunkRepo_WithTx_Enqueue.
func TestAtomicKnowledgeRepo_WithTx(t *testing.T) {
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
		// ON DELETE CASCADE takes any committed chunks/facts with it too.
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
	chunk := newTestDocumentChunk(doc.ID, 0)
	if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
		t.Fatalf("unexpected error creating parent chunk: %v", err)
	}

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)

	t.Run("Success_CommitsFactsAndMarksChunk", func(t *testing.T) {
		fact := newTestAtomicKnowledge(doc.ID, chunk.ID)

		err := knowledgeRepo.WithTx(ctx, func(tx data.AtomicKnowledgeRepoer) error {
			if err := tx.CreateBatch(ctx, []model.AtomicKnowledge{fact}); err != nil {
				return err
			}
			return tx.MarkChunkKnowledgeExtracted(ctx, chunk.ID)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		row := pool.QueryRow(
			ctx,
			"select count(*) from atomic_knowledge where document_chunk_id = $1",
			chunk.ID,
		)
		if err := row.Scan(&count); err != nil {
			t.Fatalf("unexpected error counting facts: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 fact persisted, got %d", count)
		}

		got, err := chunkRepo.GetByID(ctx, chunk.ID)
		if err != nil {
			t.Fatalf("unexpected error fetching chunk: %v", err)
		}
		if got.KnowledgeExtractedAt == nil {
			t.Error("expected KnowledgeExtractedAt to be set after extraction")
		}
	})

	t.Run("Failure_RollsBackBothFactsAndMark", func(t *testing.T) {
		chunk2 := newTestDocumentChunk(doc.ID, 1)
		if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk2}); err != nil {
			t.Fatalf("unexpected error creating second chunk: %v", err)
		}
		fact := newTestAtomicKnowledge(doc.ID, chunk2.ID)

		err := knowledgeRepo.WithTx(ctx, func(tx data.AtomicKnowledgeRepoer) error {
			if err := tx.CreateBatch(ctx, []model.AtomicKnowledge{fact}); err != nil {
				return err
			}
			return errors.New("boom")
		})
		if err == nil {
			t.Fatal("expected an error from WithTx")
		}

		var count int
		row := pool.QueryRow(
			ctx,
			"select count(*) from atomic_knowledge where document_chunk_id = $1",
			chunk2.ID,
		)
		if err := row.Scan(&count); err != nil {
			t.Fatalf("unexpected error counting facts: %v", err)
		}
		if count != 0 {
			t.Errorf("expected the fact insert to be rolled back, got %d rows", count)
		}

		got, err := chunkRepo.GetByID(ctx, chunk2.ID)
		if err != nil {
			t.Fatalf("unexpected error fetching chunk2: %v", err)
		}
		if got.KnowledgeExtractedAt != nil {
			t.Error("expected KnowledgeExtractedAt to remain nil after rollback")
		}
	})
}

func TestAtomicKnowledgeRepo_CreateBatch_Empty_NoOp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	defer pool.Close()

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
	if err := knowledgeRepo.CreateBatch(t.Context(), nil); err != nil {
		t.Errorf("expected no error for an empty batch, got %v", err)
	}
}

func TestAtomicKnowledgeRepo_ListNeedingEmbedding_FiltersToUnembedded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, _ := testhelpers.SetupTestDB(t)
	defer pool.Close()

	testhelpers.WithTxRollback(t, pool, func(ctx context.Context, q *db.Queries) {
		documentRepo := repo.NewDocumentRepo(q, pool, nil)
		doc := newTestDocument(uuid.NewString())
		if err := documentRepo.Create(ctx, doc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
		chunk := newTestDocumentChunk(doc.ID, 0)
		if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
			t.Fatalf("unexpected error creating chunk: %v", err)
		}

		knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
		needsEmbedding := newTestAtomicKnowledge(doc.ID, chunk.ID)
		alreadyEmbedded := newTestAtomicKnowledge(doc.ID, chunk.ID)
		if err := knowledgeRepo.CreateBatch(
			ctx,
			[]model.AtomicKnowledge{needsEmbedding, alreadyEmbedded},
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := knowledgeRepo.UpdateEmbedding(
			ctx,
			alreadyEmbedded.ID,
			testEmbedding(0.2),
		); err != nil {
			t.Fatalf("unexpected error embedding fact: %v", err)
		}

		got, err := knowledgeRepo.ListNeedingEmbedding(ctx, doc.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected exactly 1 fact needing embedding, got %d: %+v", len(got), got)
		}
		if got[0].ID != needsEmbedding.ID {
			t.Errorf(
				"expected the unembedded fact %s, got %s",
				needsEmbedding.ID, got[0].ID,
			)
		}
	})
}

// TestAtomicKnowledgeRepo_UpdateEmbedding_PersistsAndRoundTrips verifies the
// stored vector via a raw query against pool, so it cannot use
// testhelpers.WithTxRollback — that leaves writes uncommitted, invisible to
// a query issued over a separate pool connection. Same reasoning as
// TestAtomicKnowledgeRepo_WithTx/TestDocumentChunkRepo_WithTx_Enqueue.
func TestAtomicKnowledgeRepo_UpdateEmbedding_PersistsAndRoundTrips(t *testing.T) {
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
		// ON DELETE CASCADE takes the chunk and fact with it too.
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
	chunk := newTestDocumentChunk(doc.ID, 0)
	if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
		t.Fatalf("unexpected error creating chunk: %v", err)
	}

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
	fact := newTestAtomicKnowledge(doc.ID, chunk.ID)
	if err := knowledgeRepo.CreateBatch(ctx, []model.AtomicKnowledge{fact}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := testEmbedding(0.3)
	if err := knowledgeRepo.UpdateEmbedding(ctx, fact.ID, want); err != nil {
		t.Fatalf("unexpected error updating embedding: %v", err)
	}

	stillNeeding, err := knowledgeRepo.ListNeedingEmbedding(ctx, doc.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stillNeeding) != 0 {
		t.Fatalf(
			"expected the embedded fact to no longer need embedding, got %d: %+v",
			len(stillNeeding), stillNeeding,
		)
	}

	var roundTripped pgvector.Vector
	row := pool.QueryRow(ctx, "select embedding from atomic_knowledge where id = $1", fact.ID)
	if err := row.Scan(&roundTripped); err != nil {
		t.Fatalf("unexpected error reading back embedding: %v", err)
	}
	got := roundTripped.Slice()
	if len(got) != len(want) {
		t.Fatalf("expected embedding of length %d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected embedding %v, got %v", want, got)
			break
		}
	}
}

// setupSearchTestFixtures creates a document + a single parent chunk, real-
// committed (not testhelpers.WithTxRollback — the three SearchBy* methods
// query via r.pool directly with raw SQL, not the tx-scoped db.Queries, so
// rows inserted inside a rolled-back transaction on a different connection
// would never be visible to them; same reasoning as TestAtomicKnowledgeRepo_WithTx).
// Registers cleanup to delete the document (cascades to chunks/facts).
func setupSearchTestFixtures(
	t *testing.T,
	pool *pgxpool.Pool,
	q *db.Queries,
) (ctx context.Context, chunkID uuid.UUID) {
	t.Helper()
	ctx = t.Context()

	documentRepo := repo.NewDocumentRepo(q, pool, nil)
	doc := newTestDocument(uuid.NewString())
	if err := documentRepo.Create(ctx, doc); err != nil {
		t.Fatalf("unexpected error creating parent document: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "delete from documents where id = $1", doc.ID)
	})

	chunkRepo := repo.NewDocumentChunkRepo(q, pool, nil)
	chunk := newTestDocumentChunk(doc.ID, 0)
	if err := chunkRepo.CreateBatch(ctx, []model.DocumentChunk{chunk}); err != nil {
		t.Fatalf("unexpected error creating parent chunk: %v", err)
	}
	return ctx, chunk.ID
}

func TestAtomicKnowledgeRepo_SearchByEmbedding_OrdersByDistanceAndExcludesCurated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	t.Cleanup(func() { pool.Close() })
	ctx, chunkID := setupSearchTestFixtures(t, pool, q)

	docID, err := documentIDForChunk(ctx, pool, chunkID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
	near := newTestAtomicKnowledge(docID, chunkID)
	far := newTestAtomicKnowledge(docID, chunkID)
	invalid := newTestAtomicKnowledge(docID, chunkID)
	invalid.MarkedAsInvalid = true
	metadata := newTestAtomicKnowledge(docID, chunkID)
	metadata.Category = model.FactCategoryMetadata
	if err := knowledgeRepo.CreateBatch(
		ctx,
		[]model.AtomicKnowledge{near, far, invalid, metadata},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nearVec := testDirectionalEmbedding(0)
	farVec := testDirectionalEmbedding(500)
	for id, vec := range map[uuid.UUID][]float32{
		near.ID: nearVec, far.ID: farVec, invalid.ID: nearVec, metadata.ID: nearVec,
	} {
		if err := knowledgeRepo.UpdateEmbedding(ctx, id, vec); err != nil {
			t.Fatalf("unexpected error embedding fact: %v", err)
		}
	}

	// See TestDocumentChunkRepo_SearchByEmbedding_OrdersByDistance for why a
	// generous limit + presence/relative-order assertions are used instead
	// of an exact result count: this DB may carry real facts from unrelated
	// documents.
	results, err := knowledgeRepo.SearchByEmbedding(ctx, nearVec, 1000)
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
		case invalid.ID:
			t.Errorf("expected the marked-invalid fact to be excluded from results")
		case metadata.ID:
			t.Errorf("expected the metadata-category fact to be excluded from results")
		}
	}
	if nearIdx == -1 {
		t.Fatalf("expected the near fact to appear in results")
	}
	if farIdx == -1 {
		t.Fatalf("expected the far fact to appear in results")
	}
	if nearIdx >= farIdx {
		t.Errorf("expected near fact (rank %d) to outrank far fact (rank %d)", nearIdx, farIdx)
	}
}

func TestAtomicKnowledgeRepo_SearchByFullText_MatchesAndExcludesCurated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	t.Cleanup(func() { pool.Close() })
	ctx, chunkID := setupSearchTestFixtures(t, pool, q)

	docID, err := documentIDForChunk(ctx, pool, chunkID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
	relevant := newTestAtomicKnowledge(docID, chunkID)
	relevant.Subject = "quokka"
	relevant.Predicate = "inhabits"
	relevant.Object = "Rottnest Island"
	irrelevant := newTestAtomicKnowledge(docID, chunkID)
	irrelevant.Subject = "Alice"
	irrelevant.Predicate = "founded"
	irrelevant.Object = "Acme Corp"
	irrelevant.Note = "unrelated fact"
	irrelevantFlagged := newTestAtomicKnowledge(docID, chunkID)
	irrelevantFlagged.Subject = "quokka"
	irrelevantFlagged.Predicate = "inhabits"
	irrelevantFlagged.Object = "Rottnest Island"
	irrelevantFlagged.MarkedAsIrrelevant = true
	metadata := newTestAtomicKnowledge(docID, chunkID)
	metadata.Subject = "quokka"
	metadata.Predicate = "inhabits"
	metadata.Object = "Rottnest Island"
	metadata.Category = model.FactCategoryMetadata
	if err := knowledgeRepo.CreateBatch(
		ctx,
		[]model.AtomicKnowledge{relevant, irrelevant, irrelevantFlagged, metadata},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, err := knowledgeRepo.SearchByFullText(ctx, "quokka Rottnest", 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sawRelevant bool
	for _, r := range results {
		switch r.Item.ID {
		case relevant.ID:
			sawRelevant = true
		case irrelevant.ID:
			t.Errorf("expected the unrelated fact not to match \"quokka Rottnest\"")
		case irrelevantFlagged.ID:
			t.Errorf("expected the marked-irrelevant fact to be excluded from results")
		case metadata.ID:
			t.Errorf("expected the metadata-category fact to be excluded from results")
		}
	}
	if !sawRelevant {
		t.Fatalf("expected the quokka fact to appear in results")
	}
}

func TestAtomicKnowledgeRepo_SearchBySimilarity_MatchesAndExcludesCurated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool, q := testhelpers.SetupTestDB(t)
	t.Cleanup(func() { pool.Close() })
	ctx, chunkID := setupSearchTestFixtures(t, pool, q)

	docID, err := documentIDForChunk(ctx, pool, chunkID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	knowledgeRepo := repo.NewAtomicKnowledgeRepo(q, pool)
	similar := newTestAtomicKnowledge(docID, chunkID)
	similar.Subject = "platypus"
	dissimilar := newTestAtomicKnowledge(docID, chunkID)
	dissimilar.Subject = "spreadsheet formula"
	invalidButSimilar := newTestAtomicKnowledge(docID, chunkID)
	invalidButSimilar.Subject = "platypus"
	invalidButSimilar.MarkedAsInvalid = true
	metadataButSimilar := newTestAtomicKnowledge(docID, chunkID)
	metadataButSimilar.Subject = "platypus"
	metadataButSimilar.Category = model.FactCategoryMetadata
	if err := knowledgeRepo.CreateBatch(
		ctx,
		[]model.AtomicKnowledge{similar, dissimilar, invalidButSimilar, metadataButSimilar},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "platypuss" is a one-character typo of "platypus" — trigram-similar
	// enough to match via pg_trgm's % operator without being an exact match.
	results, err := knowledgeRepo.SearchBySimilarity(ctx, "platypuss", 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sawSimilar bool
	for _, r := range results {
		switch r.Item.ID {
		case similar.ID:
			sawSimilar = true
		case dissimilar.ID:
			t.Errorf("expected the dissimilar fact not to trigram-match \"platypuss\"")
		case invalidButSimilar.ID:
			t.Errorf("expected the marked-invalid fact to be excluded from results")
		case metadataButSimilar.ID:
			t.Errorf("expected the metadata-category fact to be excluded from results")
		}
	}
	if !sawSimilar {
		t.Fatalf("expected the platypus fact to appear in results")
	}
}

// documentIDForChunk looks up a chunk's parent document ID directly — the
// test fixtures need it to build model.AtomicKnowledge values, and
// data.DocumentChunkRepoer has no narrower way to fetch just that.
func documentIDForChunk(
	ctx context.Context,
	pool *pgxpool.Pool,
	chunkID uuid.UUID,
) (uuid.UUID, error) {
	var documentID uuid.UUID
	err := pool.QueryRow(
		ctx,
		"select document_id from document_chunks where id = $1",
		chunkID,
	).Scan(&documentID)
	return documentID, err
}
