package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// stubCaptioner is a hand-rolled data.ImageCaptioner for testing without a
// real VLM provider.
type stubCaptioner struct {
	caption string
	err     error
}

func (s *stubCaptioner) Caption(_ context.Context, _ *data.CaptionRequest) (string, error) {
	return s.caption, s.err
}

// capturingCaptioner records the CaptionRequest it was called with.
type capturingCaptioner struct {
	result string
	err    error
	got    *data.CaptionRequest
}

func (c *capturingCaptioner) Caption(_ context.Context, req *data.CaptionRequest) (string, error) {
	*c.got = *req
	return c.result, c.err
}

// newDocumentServiceWithTitle returns a *service.DocumentService whose
// GetByID always succeeds with the given title — for asserting that
// CaptionImageChunksWorker propagates DocumentTitle into CaptionRequest.
func newDocumentServiceWithTitle(title string) *service.DocumentService {
	repo := &mockrepo.DocumentRepoerMock{}
	repo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.Document, error) {
		return &model.Document{ID: id, Title: title}, nil
	}
	return service.NewDocumentService(repo)
}

func newCaptionJob(
	documentID uuid.UUID,
	chunkIDs ...uuid.UUID,
) *river.Job[queue.CaptionImageChunksDTO] {
	return &river.Job[queue.CaptionImageChunksDTO]{
		Args: queue.CaptionImageChunksDTO{DocumentID: documentID, ChunkIDs: chunkIDs},
	}
}

func TestCaptionImageChunksWorker_Work_Success(t *testing.T) {
	chunkID := uuid.New()
	chunk := model.DocumentChunk{
		ID:          chunkID,
		ParsedChunk: model.ParsedChunk{AssetURL: "document_images/abc.png", Page: 5},
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListByDocumentIDFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}
	var updatedText string
	chunkRepo.UpdateFunc = func(_ context.Context, c *model.DocumentChunk) error {
		updatedText = c.Text
		return nil
	}
	var enqueued []queue.BackgroundJob
	chunkRepo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = append(enqueued, payload)
		return nil
	}

	rawStore := &stubRawStore{
		getObjectFunc: func(_ context.Context, key string) (io.ReadCloser, error) {
			if key != chunk.AssetURL {
				t.Errorf("expected to fetch %q, got %q", chunk.AssetURL, key)
			}
			return io.NopCloser(io.LimitReader(nil, 0)), nil
		},
	}

	var gotReq data.CaptionRequest
	w := &CaptionImageChunksWorker{
		RawStore:        rawStore,
		ChunkRepo:       chunkRepo,
		DocumentService: newDocumentServiceWithTitle("My Document"),
		Captioner:       &capturingCaptioner{result: "a detailed caption", got: &gotReq},
	}

	job := newCaptionJob(uuid.New(), chunkID)
	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedText != "a detailed caption" {
		t.Errorf("expected chunk Text to be set to the caption, got %q", updatedText)
	}
	if gotReq.ContentType != "image/png" {
		t.Errorf(
			"expected content type image/png inferred from .png key, got %q",
			gotReq.ContentType,
		)
	}
	if gotReq.DocumentTitle != "My Document" {
		t.Errorf("expected document title %q, got %q", "My Document", gotReq.DocumentTitle)
	}
	if gotReq.Page != 5 {
		t.Errorf("expected page 5, got %d", gotReq.Page)
	}
	if len(enqueued) != 3 {
		t.Fatalf(
			"expected 3 jobs enqueued (embed + extract + translate), got %d: %+v",
			len(enqueued), enqueued,
		)
	}
	embedJob, ok := enqueued[0].(queue.EmbedChunksDTO)
	if !ok {
		t.Fatalf("expected first enqueued job to be queue.EmbedChunksDTO, got %T", enqueued[0])
	}
	if embedJob.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected enqueued embed job DocumentID %s, got %s",
			job.Args.DocumentID,
			embedJob.DocumentID,
		)
	}
	extractJob, ok := enqueued[1].(queue.ExtractKnowledgeDTO)
	if !ok {
		t.Fatalf(
			"expected second enqueued job to be queue.ExtractKnowledgeDTO, got %T",
			enqueued[1],
		)
	}
	if extractJob.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected enqueued extract job DocumentID %s, got %s",
			job.Args.DocumentID,
			extractJob.DocumentID,
		)
	}
	translateJob, ok := enqueued[2].(queue.TranslateChunksDTO)
	if !ok {
		t.Fatalf(
			"expected third enqueued job to be queue.TranslateChunksDTO, got %T",
			enqueued[2],
		)
	}
	if translateJob.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected enqueued translate job DocumentID %s, got %s",
			job.Args.DocumentID,
			translateJob.DocumentID,
		)
	}
}

func TestCaptionImageChunksWorker_Work_SkipsAlreadyCaptionedChunk(t *testing.T) {
	chunkID := uuid.New()
	chunk := model.DocumentChunk{
		ID: chunkID,
		ParsedChunk: model.ParsedChunk{
			AssetURL: "document_images/abc.png",
			Text:     "already captioned",
		},
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListByDocumentIDFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		t.Error("Update should not be called for an already-captioned chunk")
		return nil
	}
	chunkRepo.EnqueueFunc = func(_ context.Context, _ queue.BackgroundJob) error {
		return nil
	}

	rawStore := &stubRawStore{
		getObjectFunc: func(context.Context, string) (io.ReadCloser, error) {
			t.Error("GetObject should not be called for an already-captioned chunk")
			return nil, errors.New("should not be called")
		},
	}
	captioner := &stubCaptioner{
		err: errors.New("Caption should not be called for an already-captioned chunk"),
	}

	w := &CaptionImageChunksWorker{
		RawStore:        rawStore,
		ChunkRepo:       chunkRepo,
		DocumentService: newDocumentServiceWithTitle("Doc"),
		Captioner:       captioner,
	}
	if err := w.Work(t.Context(), newCaptionJob(uuid.New(), chunkID)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCaptionImageChunksWorker_Work_CaptionerError(t *testing.T) {
	chunkID := uuid.New()
	chunk := model.DocumentChunk{
		ID:          chunkID,
		ParsedChunk: model.ParsedChunk{AssetURL: "document_images/abc.png"},
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListByDocumentIDFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return []model.DocumentChunk{chunk}, nil
	}
	chunkRepo.UpdateFunc = func(context.Context, *model.DocumentChunk) error {
		t.Error("Update should not be called when Caption fails")
		return nil
	}

	rawStore := &stubRawStore{
		getObjectFunc: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(io.LimitReader(nil, 0)), nil
		},
	}
	captioner := &stubCaptioner{err: errors.New("not yet implemented")}

	w := &CaptionImageChunksWorker{
		RawStore:        rawStore,
		ChunkRepo:       chunkRepo,
		DocumentService: newDocumentServiceWithTitle("Doc"),
		Captioner:       captioner,
	}
	if err := w.Work(t.Context(), newCaptionJob(uuid.New(), chunkID)); err == nil {
		t.Fatal("expected an error when the captioner fails")
	}
}

func TestCaptionImageChunksWorker_Work_ChunkNotFound(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListByDocumentIDFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return nil, nil
	}

	w := &CaptionImageChunksWorker{
		RawStore:        &stubRawStore{},
		ChunkRepo:       chunkRepo,
		DocumentService: newDocumentServiceWithTitle("Doc"),
		Captioner:       &stubCaptioner{},
	}
	if err := w.Work(t.Context(), newCaptionJob(uuid.New(), uuid.New())); err == nil {
		t.Fatal("expected an error when the target chunk isn't in the document's chunk list")
	}
}

// TestCaptionImageChunksWorker_Work_MoreThanBatchSize_EnqueuesContinuation is
// a regression check: a single job execution must not try to caption an
// arbitrarily large image list in one go (see captionBatchSize's comment) —
// only captionBatchSize chunks are captioned, and a continuation
// CaptionImageChunksDTO carrying just the still-pending chunk IDs is
// enqueued for the rest instead of EmbedChunksDTO/ExtractKnowledgeDTO.
func TestCaptionImageChunksWorker_Work_MoreThanBatchSize_EnqueuesContinuation(t *testing.T) {
	chunkIDs := make([]uuid.UUID, captionBatchSize+2)
	chunks := make([]model.DocumentChunk, len(chunkIDs))
	for i := range chunkIDs {
		chunkIDs[i] = uuid.New()
		chunks[i] = model.DocumentChunk{
			ID:          chunkIDs[i],
			ParsedChunk: model.ParsedChunk{AssetURL: "document_images/abc.png"},
		}
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.ListByDocumentIDFunc = func(_ context.Context, _ uuid.UUID) ([]model.DocumentChunk, error) {
		return chunks, nil
	}
	var updatedCount int
	chunkRepo.UpdateFunc = func(_ context.Context, _ *model.DocumentChunk) error {
		updatedCount++
		return nil
	}
	var enqueued []queue.BackgroundJob
	chunkRepo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = append(enqueued, payload)
		return nil
	}

	rawStore := &stubRawStore{
		getObjectFunc: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(io.LimitReader(nil, 0)), nil
		},
	}

	w := &CaptionImageChunksWorker{
		RawStore:        rawStore,
		ChunkRepo:       chunkRepo,
		DocumentService: newDocumentServiceWithTitle("Doc"),
		Captioner:       &stubCaptioner{caption: "a caption"},
	}

	job := newCaptionJob(uuid.New(), chunkIDs...)
	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updatedCount != captionBatchSize {
		t.Fatalf(
			"expected exactly %d chunks captioned (batch cap), got %d",
			captionBatchSize,
			updatedCount,
		)
	}
	if len(enqueued) != 1 {
		t.Fatalf(
			"expected exactly 1 job enqueued (continuation only), got %d: %+v",
			len(enqueued),
			enqueued,
		)
	}
	continuation, ok := enqueued[0].(queue.CaptionImageChunksDTO)
	if !ok {
		t.Fatalf("expected a queue.CaptionImageChunksDTO continuation, got %T", enqueued[0])
	}
	if continuation.DocumentID != job.Args.DocumentID {
		t.Errorf(
			"expected continuation DocumentID %s, got %s",
			job.Args.DocumentID,
			continuation.DocumentID,
		)
	}
	if len(continuation.ChunkIDs) != 2 {
		t.Fatalf(
			"expected 2 still-pending chunk IDs in the continuation, got %d",
			len(continuation.ChunkIDs),
		)
	}
}

func TestGatherSurroundingText_IncludesSamePageParagraph(t *testing.T) {
	target := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Page: 5}}
	sibling := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page: 5, LayoutType: model.LayoutTypeParagraph, Text: "nearby paragraph",
		},
	}

	got := gatherSurroundingText([]model.DocumentChunk{target, sibling}, &target)
	if got != "nearby paragraph" {
		t.Errorf("expected %q, got %q", "nearby paragraph", got)
	}
}

func TestGatherSurroundingText_ExcludesOutOfWindowPage(t *testing.T) {
	target := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Page: 5}}
	farAway := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page: 10, LayoutType: model.LayoutTypeParagraph, Text: "far away text",
		},
	}

	got := gatherSurroundingText([]model.DocumentChunk{target, farAway}, &target)
	if got != "" {
		t.Errorf("expected empty context, got %q", got)
	}
}

func TestGatherSurroundingText_ExcludesNonContextualLayoutTypes(t *testing.T) {
	target := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Page: 5}}
	table := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page: 5, LayoutType: model.LayoutTypeTable, Text: "table content",
		},
	}
	footer := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page: 5, LayoutType: model.LayoutTypePageFooter, Text: "page 5 of 10",
		},
	}

	got := gatherSurroundingText([]model.DocumentChunk{target, table, footer}, &target)
	if got != "" {
		t.Errorf("expected empty context (table/footer excluded), got %q", got)
	}
}

func TestGatherSurroundingText_PrioritizesCaptionOverBudget(t *testing.T) {
	target := model.DocumentChunk{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Page: 5}}
	longParagraph := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page:       5,
			LayoutType: model.LayoutTypeParagraph,
			Text:       strings.Repeat("x", surroundingTextBudget),
		},
	}
	caption := model.DocumentChunk{
		ID: uuid.New(),
		ParsedChunk: model.ParsedChunk{
			Page: 5, LayoutType: model.LayoutTypeCaption, Text: "Figure 3: important caption",
		},
	}

	// The caption appears after the budget-filling paragraph in slice order,
	// but must still survive since captions are gathered in a first pass.
	got := gatherSurroundingText([]model.DocumentChunk{target, longParagraph, caption}, &target)
	if !strings.Contains(got, "Figure 3: important caption") {
		t.Errorf("expected the caption text to survive budget truncation, got %q", got)
	}
}
