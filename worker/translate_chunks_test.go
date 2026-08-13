package worker

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// stubTranslator is a hand-rolled data.Translator for testing without a real
// translation provider.
type stubTranslator struct {
	mu        sync.Mutex
	calls     []string
	translate func(text string) (string, error)
}

func (s *stubTranslator) Translate(_ context.Context, text string, _ model.Language) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, text)
	s.mu.Unlock()
	if s.translate != nil {
		return s.translate(text)
	}
	return "translated: " + text, nil
}

func (s *stubTranslator) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func newTranslateJob(documentID uuid.UUID) *river.Job[queue.TranslateChunksDTO] {
	return &river.Job[queue.TranslateChunksDTO]{
		JobRow: &rivertype.JobRow{ID: 1},
		Args:   queue.TranslateChunksDTO{DocumentID: documentID},
	}
}

func TestTranslateChunksWorker_Work_Success(t *testing.T) {
	docID := uuid.New()
	chunk1 := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "hello"}}
	chunk2 := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "world"}}

	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return &model.Document{ID: docID, Language: model.LanguageEnglish}, nil
	}

	var mu sync.Mutex
	var updated []uuid.UUID
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk1, chunk2}, nil
	}
	chunkRepo.UpdateChunkTranslationFunc = func(_ context.Context, chunkID uuid.UUID, _ string) error {
		mu.Lock()
		updated = append(updated, chunkID)
		mu.Unlock()
		return nil
	}

	translator := &stubTranslator{}
	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), translator, 0)

	if err := w.Work(t.Context(), newTranslateJob(docID)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if translator.callCount() != 2 {
		t.Errorf("expected 2 translate calls, got %d", translator.callCount())
	}
	if len(updated) != 2 {
		t.Errorf("expected 2 chunks updated, got %d", len(updated))
	}
}

func TestTranslateChunksWorker_Work_DocumentFetchError(t *testing.T) {
	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return nil, errors.New("document not found")
	}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}

	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), &stubTranslator{}, 0)

	if err := w.Work(t.Context(), newTranslateJob(uuid.New())); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestTranslateChunksWorker_Work_ListChunksError(t *testing.T) {
	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return &model.Document{Language: model.LanguageEnglish}, nil
	}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		return nil, errors.New("db unavailable")
	}

	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), &stubTranslator{}, 0)

	if err := w.Work(t.Context(), newTranslateJob(uuid.New())); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestTranslateChunksWorker_Work_TranslateErrorFailsJob(t *testing.T) {
	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return &model.Document{Language: model.LanguageEnglish}, nil
	}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "x"}}}, nil
	}

	translator := &stubTranslator{translate: func(string) (string, error) {
		return "", errors.New("translation provider down")
	}}
	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), translator, 0)

	if err := w.Work(t.Context(), newTranslateJob(uuid.New())); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestTranslateChunksWorker_Work_EnqueuesContinuationWhenBatchExceeded(t *testing.T) {
	docID := uuid.New()
	// One more chunk than translateChunksBatchSize so a continuation job is enqueued.
	chunks := make([]model.DocumentChunk, translateChunksBatchSize+1)
	for i := range chunks {
		chunks[i] = model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "chunk"}}
	}

	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return &model.Document{ID: docID, Language: model.LanguageEnglish}, nil
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}
	chunkRepo.UpdateChunkTranslationFunc = func(context.Context, uuid.UUID, string) error {
		return nil
	}
	var enqueued []queue.BackgroundJob
	chunkRepo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = append(enqueued, payload)
		return nil
	}

	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), &stubTranslator{}, 0)

	if err := w.Work(t.Context(), newTranslateJob(docID)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(enqueued) != 1 {
		t.Fatalf("expected 1 continuation job enqueued, got %d", len(enqueued))
	}
	continuation, ok := enqueued[0].(queue.TranslateChunksDTO)
	if !ok {
		t.Fatalf("enqueued job is %T, want queue.TranslateChunksDTO", enqueued[0])
	}
	if continuation.DocumentID != docID {
		t.Errorf("continuation DocumentID = %v, want %v", continuation.DocumentID, docID)
	}
}

func TestTranslateChunksWorker_Work_ContinuationEnqueueError(t *testing.T) {
	docID := uuid.New()
	chunks := make([]model.DocumentChunk, translateChunksBatchSize+1)
	for i := range chunks {
		chunks[i] = model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "chunk"}}
	}

	docRepo := &mockrepo.DocumentRepoerMock{}
	docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
		return &model.Document{ID: docID, Language: model.LanguageEnglish}, nil
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}
	chunkRepo.UpdateChunkTranslationFunc = func(context.Context, uuid.UUID, string) error {
		return nil
	}
	chunkRepo.EnqueueFunc = func(context.Context, queue.BackgroundJob) error {
		return errors.New("queue unavailable")
	}

	w := NewTranslateChunksWorker(chunkRepo, service.NewDocumentService(docRepo), &stubTranslator{}, 0)

	if err := w.Work(t.Context(), newTranslateJob(docID)); err == nil {
		t.Fatal("expected an error from the continuation enqueue failure, got nil")
	}
}

func TestTranslateChunksWorker_concurrency(t *testing.T) {
	tests := []struct {
		name string
		set  int
		want int
	}{
		{"unset falls back to default", 0, translateChunksDefaultConcurrency},
		{"negative falls back to default", -1, translateChunksDefaultConcurrency},
		{"positive value used as-is", 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &TranslateChunksWorker{Concurrency: tt.set}
			if got := w.concurrency(); got != tt.want {
				t.Errorf("concurrency() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTranslateChunksWorker_Timeout(t *testing.T) {
	t.Run("falls back when sizing query errors", func(t *testing.T) {
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
			return nil, errors.New("db unavailable")
		}
		w := &TranslateChunksWorker{ChunkRepo: chunkRepo}

		if got := w.Timeout(newTranslateJob(uuid.New())); got != translateChunksFallbackTimeout {
			t.Errorf("Timeout() = %v, want fallback %v", got, translateChunksFallbackTimeout)
		}
	})

	t.Run("scales with pending chunk count", func(t *testing.T) {
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListChunksNeedingTranslationFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
			return make([]model.DocumentChunk, 3), nil
		}
		w := &TranslateChunksWorker{ChunkRepo: chunkRepo, Concurrency: 3}

		want := translateChunksFixedOverhead + translateChunksPerChunkBudget // 3 chunks / concurrency 3 = 1 round
		if got := w.Timeout(newTranslateJob(uuid.New())); got != want {
			t.Errorf("Timeout() = %v, want %v", got, want)
		}
	})
}
