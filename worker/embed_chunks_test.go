package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/riverqueue/river"
)

// stubEmbedder is a hand-rolled data.Embedder for testing without a real
// embeddings provider.
type stubEmbedder struct {
	// embed, if set, computes the returned vectors; overrides fixed/err.
	embed func(texts []string) ([][]float32, error)
	fixed [][]float32
	err   error
	calls [][]string
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls = append(s.calls, texts)
	if s.embed != nil {
		return s.embed(texts)
	}
	return s.fixed, s.err
}

func newEmbedJob(documentID uuid.UUID) *river.Job[queue.EmbedChunksDTO] {
	return &river.Job[queue.EmbedChunksDTO]{
		Args: queue.EmbedChunksDTO{DocumentID: documentID},
	}
}

func TestEmbedChunksWorker_Work_Success(t *testing.T) {
	chunk1 := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "first"}}
	chunk2 := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "second"}}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk1, chunk2}, nil
	}
	var updated []model.DocumentChunk
	chunkRepo.UpdateFunc = func(_ context.Context, c *model.DocumentChunk) error {
		updated = append(updated, *c)
		return nil
	}

	embedder := &stubEmbedder{
		fixed: [][]float32{{0.1, 0.2}, {0.3, 0.4}},
	}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updated) != 2 {
		t.Fatalf("expected 2 chunks updated, got %d", len(updated))
	}
	if updated[0].ID != chunk1.ID || len(updated[0].Embedding) != 2 ||
		updated[0].Embedding[0] != 0.1 {
		t.Errorf("expected chunk1 embedded with [0.1 0.2], got %+v", updated[0])
	}
	if updated[1].ID != chunk2.ID || len(updated[1].Embedding) != 2 ||
		updated[1].Embedding[0] != 0.3 {
		t.Errorf("expected chunk2 embedded with [0.3 0.4], got %+v", updated[1])
	}
	if len(embedder.calls) != 1 || len(embedder.calls[0]) != 2 {
		t.Errorf("expected a single batched Embed call with 2 texts, got %v", embedder.calls)
	}
}

func TestEmbedChunksWorker_Work_NoChunksNeedingEmbedding_NoOp(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return nil, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		t.Error("Update should not be called when there are no chunks needing embedding")
		return nil
	}
	embedder := &stubEmbedder{
		embed: func([]string) ([][]float32, error) {
			t.Error("Embed should not be called when there are no chunks needing embedding")
			return nil, nil
		},
	}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbedChunksWorker_Work_BatchesAcrossEmbedBatchSize(t *testing.T) {
	const total = embedBatchSize + 5
	chunks := make([]model.DocumentChunk, total)
	for i := range chunks {
		chunks[i] = model.DocumentChunk{
			ID:          uuid.New(),
			ParsedChunk: model.ParsedChunk{Text: "text"},
		}
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}
	updateCalls := 0
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		updateCalls++
		return nil
	}

	embedder := &stubEmbedder{
		embed: func(texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i := range vecs {
				vecs[i] = []float32{1}
			}
			return vecs, nil
		},
	}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embedder.calls) != 2 {
		t.Fatalf("expected 2 batched Embed calls, got %d", len(embedder.calls))
	}
	if len(embedder.calls[0]) != embedBatchSize {
		t.Errorf("expected first batch of %d, got %d", embedBatchSize, len(embedder.calls[0]))
	}
	if len(embedder.calls[1]) != total-embedBatchSize {
		t.Errorf(
			"expected second batch of %d, got %d",
			total-embedBatchSize,
			len(embedder.calls[1]),
		)
	}
	if updateCalls != total {
		t.Errorf("expected %d Update calls, got %d", total, updateCalls)
	}
}

func TestEmbedChunksWorker_Work_EmbedderError(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{
			{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "text"}},
		}, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		t.Error("Update should not be called when Embed fails")
		return nil
	}
	embedder := &stubEmbedder{err: errors.New("embeddings endpoint down")}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err == nil {
		t.Fatal("expected an error when the embedder fails")
	}
}

func TestEmbedChunksWorker_Work_MismatchedVectorCount(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{
			{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "a"}},
			{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "b"}},
		}, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		t.Error("Update should not be called when the embedder returns a mismatched vector count")
		return nil
	}
	embedder := &stubEmbedder{fixed: [][]float32{{0.1}}}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err == nil {
		t.Fatal("expected an error when the embedder returns a mismatched vector count")
	}
}

func TestEmbedChunksWorker_Work_UpdateError(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{
			{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "text"}},
		}, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		return errors.New("db down")
	}
	embedder := &stubEmbedder{fixed: [][]float32{{0.1}}}

	w := NewEmbedChunksWorker(chunkRepo, embedder)
	if err := w.Work(t.Context(), newEmbedJob(uuid.New())); err == nil {
		t.Fatal("expected an error when Update fails")
	}
}
