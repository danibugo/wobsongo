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

func newEmbedKnowledgeJob(documentID uuid.UUID) *river.Job[queue.EmbedKnowledgeDTO] {
	return &river.Job[queue.EmbedKnowledgeDTO]{
		Args: queue.EmbedKnowledgeDTO{DocumentID: documentID},
	}
}

func TestEmbedKnowledgeWorker_Work_Success(t *testing.T) {
	fact1 := model.AtomicKnowledge{
		ID:        uuid.New(),
		Subject:   "Alice",
		Predicate: "founded",
		Object:    "Acme",
	}
	fact2 := model.AtomicKnowledge{
		ID:        uuid.New(),
		Subject:   "Bob",
		Predicate: "leads",
		Object:    "Sales",
		Note:      "as of 2020",
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.ListNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.AtomicKnowledge, error) {
		return []model.AtomicKnowledge{fact1, fact2}, nil
	}
	var updated []struct {
		id        uuid.UUID
		embedding []float32
	}
	knowledgeRepo.UpdateEmbeddingFunc = func(_ context.Context, id uuid.UUID, embedding []float32) error {
		updated = append(updated, struct {
			id        uuid.UUID
			embedding []float32
		}{id, embedding})
		return nil
	}

	embedder := &stubEmbedder{
		embed: func(texts []string) ([][]float32, error) {
			if len(texts) != 2 {
				t.Fatalf("expected 2 texts, got %d", len(texts))
			}
			if texts[0] != "Alice founded Acme" {
				t.Errorf("expected fact1 text %q, got %q", "Alice founded Acme", texts[0])
			}
			if texts[1] != "Bob leads Sales as of 2020" {
				t.Errorf("expected fact2 text %q, got %q", "Bob leads Sales as of 2020", texts[1])
			}
			return [][]float32{{0.1}, {0.2}}, nil
		},
	}

	w := NewEmbedKnowledgeWorker(knowledgeRepo, embedder)
	if err := w.Work(t.Context(), newEmbedKnowledgeJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updated) != 2 {
		t.Fatalf("expected 2 facts updated, got %d", len(updated))
	}
	if updated[0].id != fact1.ID || updated[0].embedding[0] != 0.1 {
		t.Errorf("unexpected update for fact1: %+v", updated[0])
	}
	if updated[1].id != fact2.ID || updated[1].embedding[0] != 0.2 {
		t.Errorf("unexpected update for fact2: %+v", updated[1])
	}
}

func TestEmbedKnowledgeWorker_Work_NoFactsNeedingEmbedding_NoOp(t *testing.T) {
	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.ListNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.AtomicKnowledge, error) {
		return nil, nil
	}
	knowledgeRepo.UpdateEmbeddingFunc = func(context.Context, uuid.UUID, []float32) error {
		t.Error("UpdateEmbedding should not be called when there are no facts needing embedding")
		return nil
	}
	embedder := &stubEmbedder{
		embed: func([]string) ([][]float32, error) {
			t.Error("Embed should not be called when there are no facts needing embedding")
			return nil, nil
		},
	}

	w := NewEmbedKnowledgeWorker(knowledgeRepo, embedder)
	if err := w.Work(t.Context(), newEmbedKnowledgeJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbedKnowledgeWorker_Work_BatchesAcrossEmbedBatchSize(t *testing.T) {
	const total = embedBatchSize + 3
	facts := make([]model.AtomicKnowledge, total)
	for i := range facts {
		facts[i] = model.AtomicKnowledge{ID: uuid.New(), Subject: "s", Predicate: "p", Object: "o"}
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.ListNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.AtomicKnowledge, error) {
		return facts, nil
	}
	updateCalls := 0
	knowledgeRepo.UpdateEmbeddingFunc = func(context.Context, uuid.UUID, []float32) error {
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

	w := NewEmbedKnowledgeWorker(knowledgeRepo, embedder)
	if err := w.Work(t.Context(), newEmbedKnowledgeJob(uuid.New())); err != nil {
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
		t.Errorf("expected %d UpdateEmbedding calls, got %d", total, updateCalls)
	}
}

func TestEmbedKnowledgeWorker_Work_EmbedderError(t *testing.T) {
	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.ListNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.AtomicKnowledge, error) {
		return []model.AtomicKnowledge{
			{ID: uuid.New(), Subject: "s", Predicate: "p", Object: "o"},
		}, nil
	}
	knowledgeRepo.UpdateEmbeddingFunc = func(context.Context, uuid.UUID, []float32) error {
		t.Error("UpdateEmbedding should not be called when Embed fails")
		return nil
	}
	embedder := &stubEmbedder{err: errors.New("embeddings endpoint down")}

	w := NewEmbedKnowledgeWorker(knowledgeRepo, embedder)
	if err := w.Work(t.Context(), newEmbedKnowledgeJob(uuid.New())); err == nil {
		t.Fatal("expected an error when the embedder fails")
	}
}

func TestEmbedKnowledgeWorker_Work_UpdateEmbeddingError(t *testing.T) {
	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.ListNeedingEmbeddingFunc = func(_ context.Context, _ uuid.UUID) ([]model.AtomicKnowledge, error) {
		return []model.AtomicKnowledge{
			{ID: uuid.New(), Subject: "s", Predicate: "p", Object: "o"},
		}, nil
	}
	knowledgeRepo.UpdateEmbeddingFunc = func(context.Context, uuid.UUID, []float32) error {
		return errors.New("db down")
	}
	embedder := &stubEmbedder{fixed: [][]float32{{0.1}}}

	w := NewEmbedKnowledgeWorker(knowledgeRepo, embedder)
	if err := w.Work(t.Context(), newEmbedKnowledgeJob(uuid.New())); err == nil {
		t.Fatal("expected an error when UpdateEmbedding fails")
	}
}
