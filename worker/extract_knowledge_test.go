package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// newDocumentServiceWithGetByIDError returns a *service.DocumentService whose
// GetByID always fails with err.
func newDocumentServiceWithGetByIDError(err error) *service.DocumentService {
	repo := &mockrepo.DocumentRepoerMock{}
	repo.GetByIDFunc = func(_ context.Context, _ uuid.UUID) (*model.Document, error) {
		return nil, err
	}
	return service.NewDocumentService(repo)
}

// stubExtractor is a hand-rolled data.KnowledgeExtractor for testing without
// a real LLM provider.
type stubExtractor struct {
	extract func(req *data.ExtractionRequest) ([]data.ExtractedFact, error)
	facts   []data.ExtractedFact
	err     error
}

func (s *stubExtractor) Extract(
	_ context.Context,
	req *data.ExtractionRequest,
) ([]data.ExtractedFact, error) {
	if s.extract != nil {
		return s.extract(req)
	}
	return s.facts, s.err
}

func newExtractJob(documentID uuid.UUID) *river.Job[queue.ExtractKnowledgeDTO] {
	return &river.Job[queue.ExtractKnowledgeDTO]{
		Args: queue.ExtractKnowledgeDTO{DocumentID: documentID},
	}
}

// newAtomicKnowledgeRepoWithTx returns a mockrepo.AtomicKnowledgeRepoerMock
// wired so WithTx calls back into itself (the established pattern, mirroring
// newPassThroughChunkRepo in process_parsed_document_test.go).
func newAtomicKnowledgeRepoWithTx() *mockrepo.AtomicKnowledgeRepoerMock {
	repo := &mockrepo.AtomicKnowledgeRepoerMock{}
	repo.WithTxFunc = func(_ context.Context, fn func(data.AtomicKnowledgeRepoer) error) error {
		return fn(repo)
	}
	return repo
}

func TestExtractKnowledgeWorker_Work_Success(t *testing.T) {
	chunk1 := model.DocumentChunk{
		ID:          uuid.New(),
		ParsedChunk: model.ParsedChunk{Text: "Acme was founded by Alice."},
	}
	chunk2 := model.DocumentChunk{
		ID:          uuid.New(),
		ParsedChunk: model.ParsedChunk{Text: "The sky is blue."},
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk1, chunk2}, nil
	}
	var enqueued queue.BackgroundJob
	chunkRepo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = payload
		return nil
	}

	documentService := newDocumentServiceWithTitle("Company History")

	// Chunks now extract concurrently (up to the worker's Concurrency), so
	// these mock callbacks can be invoked from multiple goroutines at once.
	var mu sync.Mutex
	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	var created []model.AtomicKnowledge
	knowledgeRepo.CreateBatchFunc = func(_ context.Context, facts []model.AtomicKnowledge) error {
		mu.Lock()
		defer mu.Unlock()
		created = append(created, facts...)
		return nil
	}
	var markedChunkIDs []uuid.UUID
	knowledgeRepo.MarkChunkKnowledgeExtractedFunc = func(_ context.Context, chunkID uuid.UUID) error {
		mu.Lock()
		defer mu.Unlock()
		markedChunkIDs = append(markedChunkIDs, chunkID)
		return nil
	}

	extractor := &stubExtractor{
		extract: func(req *data.ExtractionRequest) ([]data.ExtractedFact, error) {
			if req.Text == chunk1.Text {
				return []data.ExtractedFact{
					{
						Subject:   "Alice",
						Predicate: "founded",
						Object:    "Acme",
						TruthTier: model.TruthTierAxiomatic,
					},
				}, nil
			}
			return nil, nil // chunk2 yields zero facts
		},
	}

	w := NewExtractKnowledgeWorker(chunkRepo, knowledgeRepo, documentService, extractor, 0)
	job := newExtractJob(uuid.New())
	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(created) != 1 {
		t.Fatalf("expected 1 fact created, got %d", len(created))
	}
	if created[0].Subject != "Alice" || created[0].DocumentChunkID != chunk1.ID {
		t.Errorf("unexpected created fact: %+v", created[0])
	}
	if len(markedChunkIDs) != 2 {
		t.Fatalf("expected both chunks marked extracted, got %d", len(markedChunkIDs))
	}
	// Order isn't guaranteed: chunks extract concurrently.
	markedSet := map[uuid.UUID]bool{markedChunkIDs[0]: true, markedChunkIDs[1]: true}
	if !markedSet[chunk1.ID] || !markedSet[chunk2.ID] {
		t.Errorf("expected both chunk1 and chunk2 marked extracted, got %v", markedChunkIDs)
	}

	embedJob, ok := enqueued.(queue.EmbedKnowledgeDTO)
	if !ok {
		t.Fatalf("expected a queue.EmbedKnowledgeDTO to be enqueued, got %T", enqueued)
	}
	if embedJob.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected enqueued embed job DocumentID %s, got %s",
			job.Args.DocumentID,
			embedJob.DocumentID,
		)
	}
}

// TestExtractKnowledgeWorker_Work_DiscardsMetadataCategoryFacts is a
// regression check: a metadata-category fact must never be passed to
// CreateBatch at all (not just tagged and kept) — only clinical/unknown
// facts get persisted.
func TestExtractKnowledgeWorker_Work_DiscardsMetadataCategoryFacts(t *testing.T) {
	chunk := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "text"}}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}
	chunkRepo.EnqueueFunc = func(_ context.Context, _ queue.BackgroundJob) error {
		return nil
	}

	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	var created []model.AtomicKnowledge
	knowledgeRepo.CreateBatchFunc = func(_ context.Context, facts []model.AtomicKnowledge) error {
		created = append(created, facts...)
		return nil
	}
	knowledgeRepo.MarkChunkKnowledgeExtractedFunc = func(context.Context, uuid.UUID) error {
		return nil
	}

	extractor := &stubExtractor{
		facts: []data.ExtractedFact{
			{
				Subject:   "WHO",
				Predicate: "recommends",
				Object:    "X",
				Category:  model.FactCategoryClinical,
			},
			{
				Subject:   "Alice",
				Predicate: "authored",
				Object:    "chapter 3",
				Category:  model.FactCategoryMetadata,
			},
			{
				Subject:   "Bob",
				Predicate: "might be",
				Object:    "relevant",
				Category:  model.FactCategoryUnknown,
			},
		},
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		knowledgeRepo,
		newDocumentServiceWithTitle("Doc"),
		extractor,
		0,
	)
	if err := w.Work(t.Context(), newExtractJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(created) != 2 {
		t.Fatalf(
			"expected 2 persisted facts (clinical + unknown), got %d: %+v",
			len(created),
			created,
		)
	}
	for _, f := range created {
		if f.Category == model.FactCategoryMetadata {
			t.Errorf("expected no metadata-category fact to be persisted, got %+v", f)
		}
	}
}

// TestExtractKnowledgeWorker_Work_MoreThanBatchSize_EnqueuesContinuation is a
// regression check: a single job execution must not try to extract an
// arbitrarily large backlog in one go (see extractKnowledgeBatchSize's
// comment) — only extractKnowledgeBatchSize chunks are processed, and a
// continuation ExtractKnowledgeDTO is enqueued for the rest instead of
// EmbedKnowledgeDTO.
func TestExtractKnowledgeWorker_Work_MoreThanBatchSize_EnqueuesContinuation(t *testing.T) {
	chunks := make([]model.DocumentChunk, extractKnowledgeBatchSize+3)
	for i := range chunks {
		chunks[i] = model.DocumentChunk{
			ID:          uuid.New(),
			ParsedChunk: model.ParsedChunk{Text: "text"},
		}
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}
	var enqueued queue.BackgroundJob
	chunkRepo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = payload
		return nil
	}

	var mu sync.Mutex
	var markedChunkIDs []uuid.UUID
	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	knowledgeRepo.CreateBatchFunc = func(context.Context, []model.AtomicKnowledge) error {
		return nil
	}
	knowledgeRepo.MarkChunkKnowledgeExtractedFunc = func(_ context.Context, chunkID uuid.UUID) error {
		mu.Lock()
		defer mu.Unlock()
		markedChunkIDs = append(markedChunkIDs, chunkID)
		return nil
	}

	extractor := &stubExtractor{facts: nil}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		knowledgeRepo,
		newDocumentServiceWithTitle("Doc"),
		extractor,
		0,
	)
	job := newExtractJob(uuid.New())
	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(markedChunkIDs) != extractKnowledgeBatchSize {
		t.Fatalf(
			"expected exactly %d chunks marked extracted (batch cap), got %d",
			extractKnowledgeBatchSize, len(markedChunkIDs),
		)
	}

	continuation, ok := enqueued.(queue.ExtractKnowledgeDTO)
	if !ok {
		t.Fatalf(
			"expected a queue.ExtractKnowledgeDTO continuation to be enqueued, got %T",
			enqueued,
		)
	}
	if continuation.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected continuation DocumentID %s, got %s",
			job.Args.DocumentID,
			continuation.DocumentID,
		)
	}
}

func TestExtractKnowledgeWorker_Work_NoChunksNeedingExtraction_NoOp(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return nil, nil
	}
	chunkRepo.EnqueueFunc = func(_ context.Context, _ queue.BackgroundJob) error {
		return nil
	}

	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	knowledgeRepo.CreateBatchFunc = func(context.Context, []model.AtomicKnowledge) error {
		t.Error("CreateBatch should not be called when there are no chunks needing extraction")
		return nil
	}

	extractor := &stubExtractor{
		extract: func(*data.ExtractionRequest) ([]data.ExtractedFact, error) {
			t.Error("Extract should not be called when there are no chunks needing extraction")
			return nil, nil
		},
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		knowledgeRepo,
		newDocumentServiceWithTitle("Doc"),
		extractor,
		0,
	)
	if err := w.Work(t.Context(), newExtractJob(uuid.New())); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractKnowledgeWorker_Work_ExtractorError_ChunkNotMarked(t *testing.T) {
	chunk := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "text"}}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}

	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	knowledgeRepo.MarkChunkKnowledgeExtractedFunc = func(context.Context, uuid.UUID) error {
		t.Error("MarkChunkKnowledgeExtracted should not be called when Extract fails")
		return nil
	}

	extractor := &stubExtractor{err: errors.New("llm endpoint down")}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		knowledgeRepo,
		newDocumentServiceWithTitle("Doc"),
		extractor,
		0,
	)
	if err := w.Work(t.Context(), newExtractJob(uuid.New())); err == nil {
		t.Fatal("expected an error when the extractor fails")
	}
}

func TestExtractKnowledgeWorker_Work_CreateBatchError(t *testing.T) {
	chunk := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "text"}}
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}

	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	knowledgeRepo.CreateBatchFunc = func(context.Context, []model.AtomicKnowledge) error {
		return errors.New("db down")
	}
	knowledgeRepo.MarkChunkKnowledgeExtractedFunc = func(context.Context, uuid.UUID) error {
		t.Error("MarkChunkKnowledgeExtracted should not be called when CreateBatch fails")
		return nil
	}

	extractor := &stubExtractor{
		facts: []data.ExtractedFact{{Subject: "a", Predicate: "b", Object: "c"}},
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		knowledgeRepo,
		newDocumentServiceWithTitle("Doc"),
		extractor,
		0,
	)
	if err := w.Work(t.Context(), newExtractJob(uuid.New())); err == nil {
		t.Fatal("expected an error when CreateBatch fails")
	}
}

func TestExtractKnowledgeWorker_Work_DocumentServiceError(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(context.Context, uuid.UUID) ([]model.DocumentChunk, error) {
		t.Error(
			"ListChunksNeedingKnowledgeExtraction should not be called when fetching the document fails",
		)
		return nil, nil
	}

	knowledgeRepo := newAtomicKnowledgeRepoWithTx()
	extractor := &stubExtractor{}

	documentService := newDocumentServiceWithGetByIDError(errors.New("db down"))
	w := NewExtractKnowledgeWorker(chunkRepo, knowledgeRepo, documentService, extractor, 0)
	if err := w.Work(t.Context(), newExtractJob(uuid.New())); err == nil {
		t.Fatal("expected an error when fetching the document fails")
	}
}

// TestExtractKnowledgeWorker_Timeout_ScalesWithPendingChunkCount is a
// regression check: a real job kept dying with "context deadline exceeded"
// partway through a large document under the previous flat 15-minute
// timeout — a document with enough text-bearing chunks always eventually
// exceeds any fixed budget. Timeout() must scale with the live pending-chunk
// count instead, divided into concurrent rounds (extractKnowledgeConcurrency
// chunks extract at once, not strictly one at a time).
func TestExtractKnowledgeWorker_Timeout_ScalesWithPendingChunkCount(t *testing.T) {
	// Deliberately not a multiple of the concurrency limit, so the
	// ceiling-division rounding is actually exercised.
	const concurrency = 4
	chunks := make([]model.DocumentChunk, concurrency*2+2)
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		newAtomicKnowledgeRepoWithTx(),
		newDocumentServiceWithTitle("Doc"),
		&stubExtractor{},
		concurrency,
	)
	got := w.Timeout(newExtractJob(uuid.New()))
	wantRounds := 3 // ceil((2*concurrency + 2) / concurrency)
	want := extractKnowledgeFixedOverhead + time.Duration(wantRounds)*extractKnowledgePerChunkBudget
	if got != want {
		t.Errorf("expected timeout %v for %d pending chunks, got %v", want, len(chunks), got)
	}
}

// TestExtractKnowledgeWorker_Concurrency_FallsBackToDefault confirms an
// unset/invalid Concurrency (<=0) resolves to extractKnowledgeDefaultConcurrency.
func TestExtractKnowledgeWorker_Concurrency_FallsBackToDefault(t *testing.T) {
	chunks := make([]model.DocumentChunk, extractKnowledgeDefaultConcurrency*2+2)
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		newAtomicKnowledgeRepoWithTx(),
		newDocumentServiceWithTitle("Doc"),
		&stubExtractor{},
		0,
	)
	got := w.Timeout(newExtractJob(uuid.New()))
	wantRounds := 3 // ceil((2*default + 2) / default)
	want := extractKnowledgeFixedOverhead + time.Duration(wantRounds)*extractKnowledgePerChunkBudget
	if got != want {
		t.Errorf(
			"expected timeout %v assuming default concurrency %d, got %v",
			want,
			extractKnowledgeDefaultConcurrency,
			got,
		)
	}
}

// TestExtractKnowledgeWorker_Timeout_FallsBackOnQueryError confirms a failure
// in the sizing query itself degrades to a safe, generous fallback rather
// than an unusably small or zero timeout.
func TestExtractKnowledgeWorker_Timeout_FallsBackOnQueryError(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListChunksNeedingKnowledgeExtractionFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return nil, errors.New("db down")
	}

	w := NewExtractKnowledgeWorker(
		chunkRepo,
		newAtomicKnowledgeRepoWithTx(),
		newDocumentServiceWithTitle("Doc"),
		&stubExtractor{},
		0,
	)
	got := w.Timeout(newExtractJob(uuid.New()))
	if got != extractKnowledgeFallbackTimeout {
		t.Errorf("expected fallback timeout %v, got %v", extractKnowledgeFallbackTimeout, got)
	}
}
