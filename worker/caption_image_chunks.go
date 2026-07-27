package worker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// captionPerChunkBudget bounds how long a single chunk gets within the job's
// overall timeout — margin over VLMClient's own per-call HTTP timeout (5
// minutes, external/vlm_client.go's vlmHTTPTimeout) to also cover that
// chunk's S3 fetch and DB update. Chunks are captioned sequentially (no
// concurrency here — VLM calls aren't parallelized the way extraction's are).
const captionPerChunkBudget = 6 * time.Minute

// captionImageChunksFixedOverhead covers the job's work outside the per-chunk
// loop: fetching the document and its chunk list up front, plus the two
// enqueue calls at the end.
const captionImageChunksFixedOverhead = 1 * time.Minute

// captionBatchSize caps how many chunks a single job execution captions
// before re-enqueueing a continuation job (with only the still-pending
// chunk IDs) for the rest — see extractKnowledgeBatchSize in
// extract_knowledge.go for the full reasoning (River's JobRescuer
// force-retries any job after a flat, client-wide duration, 1 hour by
// default, regardless of a worker's own Timeout()). At this batch size,
// Timeout() caps around 37 minutes (captionImageChunksFixedOverhead +
// captionBatchSize*captionPerChunkBudget), comfortably under that default.
const captionBatchSize = 6

// extensionContentTypes recovers an image's content type from its stored S3
// key's extension — the reverse of imageContentTypeExtensions
// (process_parsed_document.go), which chose that extension in the first place.
var extensionContentTypes = map[string]string{
	"png":  "image/png",
	"jpeg": contentTypeImageJPEG,
	"jpg":  contentTypeImageJPEG,
	"webp": "image/webp",
	"avif": "image/avif",
}

// contextualLayoutTypes are chunk types worth including as grounding text
// when captioning a nearby image — the parts of a document a human would
// actually read for context, excluding noise, tables, and other images.
var contextualLayoutTypes = map[model.LayoutType]bool{
	model.LayoutTypeTitle:         true,
	model.LayoutTypeSectionHeader: true,
	model.LayoutTypeParagraph:     true,
	model.LayoutTypeListItem:      true,
	model.LayoutTypeCaption:       true, // Docling's own figure/table caption — strongest signal
}

const (
	// surroundingPageWindow bounds how many pages away from an image a
	// context chunk may be and still count as "surrounding."
	surroundingPageWindow = 1

	// surroundingTextBudget caps how many characters of gathered context
	// are sent per image, to bound prompt size/cost.
	surroundingTextBudget = 2000
)

// CaptionImageChunksWorker is a River worker that generates and stores a
// caption for each image/chart chunk enqueued by ProcessParsedDocumentWorker.
// Kept as its own job (not inline in that worker) so a slow/costly/
// rate-limited VLM call never fails or retries chunk storage itself.
type CaptionImageChunksWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.CaptionImageChunksDTO]
	// RawStore fetches each chunk's image bytes back from S3.
	RawStore data.RawObjectStore
	// ChunkRepo reads chunks and saves their generated captions.
	ChunkRepo data.DocumentChunkRepoer
	// DocumentService fetches the parent document's title, for grounding.
	DocumentService *service.DocumentService
	// Captioner generates the caption text for an image.
	Captioner data.ImageCaptioner
}

// NewCaptionImageChunksWorker is a constructor for CaptionImageChunksWorker.
func NewCaptionImageChunksWorker(
	rawStore data.RawObjectStore,
	chunkRepo data.DocumentChunkRepoer,
	documentService *service.DocumentService,
	captioner data.ImageCaptioner,
) *CaptionImageChunksWorker {
	return &CaptionImageChunksWorker{
		RawStore:        rawStore,
		ChunkRepo:       chunkRepo,
		DocumentService: documentService,
		Captioner:       captioner,
	}
}

// Timeout overrides River's default job timeout, scaled to the number of
// image chunks this execution will actually attempt (bounded by
// captionBatchSize, never the full job.Args.ChunkIDs list — see that
// constant's comment for why).
func (w *CaptionImageChunksWorker) Timeout(
	job *river.Job[queue.CaptionImageChunksDTO],
) time.Duration {
	n := min(len(job.Args.ChunkIDs), captionBatchSize)
	return captionImageChunksFixedOverhead + time.Duration(n)*captionPerChunkBudget
}

// Work is the main method that gets called when a job is dequeued.
func (w *CaptionImageChunksWorker) Work(
	ctx context.Context,
	job *river.Job[queue.CaptionImageChunksDTO],
) error {
	doc, err := w.DocumentService.GetByID(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("failed to fetch document %s: %w", job.Args.DocumentID, err)
	}

	chunks, err := w.ChunkRepo.ListByDocumentID(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to list chunks for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	// Resolve still-pending (not yet captioned) chunk IDs, preserving order —
	// a prior attempt or batch may have already captioned some of
	// job.Args.ChunkIDs.
	pending := make([]uuid.UUID, 0, len(job.Args.ChunkIDs))
	for _, chunkID := range job.Args.ChunkIDs {
		idx := indexOfChunk(chunks, chunkID)
		if idx < 0 {
			return fmt.Errorf("chunk %s not found for document %s", chunkID, job.Args.DocumentID)
		}
		if chunks[idx].Text == "" {
			pending = append(pending, chunkID)
		}
	}

	// Cap this execution to captionBatchSize chunks — see that constant's
	// comment. Whatever's left gets a continuation job below.
	moreRemaining := len(pending) > captionBatchSize
	batch := pending
	if moreRemaining {
		batch = pending[:captionBatchSize]
	}

	for _, chunkID := range batch {
		chunk := &chunks[indexOfChunk(chunks, chunkID)]

		rc, err := w.RawStore.GetObject(ctx, chunk.AssetURL)
		if err != nil {
			return fmt.Errorf("failed to fetch image for chunk %s: %w", chunkID, err)
		}
		imageBytes, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("failed to read image for chunk %s: %w", chunkID, err)
		}

		caption, err := w.Captioner.Caption(ctx, &data.CaptionRequest{
			ImageBytes:      imageBytes,
			ContentType:     contentTypeFromKey(chunk.AssetURL),
			DocumentTitle:   doc.Title,
			Page:            chunk.Page,
			SurroundingText: gatherSurroundingText(chunks, chunk),
		})
		if err != nil {
			return fmt.Errorf("failed to caption image for chunk %s: %w", chunkID, err)
		}

		chunk.Text = caption
		chunk.UpdatedAt = time.Now()
		if err := w.ChunkRepo.Update(ctx, chunk); err != nil {
			return fmt.Errorf("failed to save caption for chunk %s: %w", chunkID, err)
		}
	}

	if moreRemaining {
		// More chunks remain beyond this batch — re-enqueue with just the
		// still-pending IDs rather than assuming this document is done.
		if err := w.ChunkRepo.Enqueue(ctx, queue.CaptionImageChunksDTO{
			DocumentID: job.Args.DocumentID,
			ChunkIDs:   pending[captionBatchSize:],
		}); err != nil {
			return fmt.Errorf("failed to enqueue captioning continuation: %w", err)
		}
		return nil
	}

	// Every image chunk for this document now has final text (captioned here
	// or already captioned by a prior partial attempt) — safe to embed and
	// extract knowledge.
	if err := w.ChunkRepo.Enqueue(ctx, queue.EmbedChunksDTO{
		DocumentID: job.Args.DocumentID,
	}); err != nil {
		return fmt.Errorf("failed to enqueue chunk embedding: %w", err)
	}
	if err := w.ChunkRepo.Enqueue(ctx, queue.ExtractKnowledgeDTO{
		DocumentID: job.Args.DocumentID,
	}); err != nil {
		return fmt.Errorf("failed to enqueue knowledge extraction: %w", err)
	}
	if err := w.ChunkRepo.Enqueue(ctx, queue.TranslateChunksDTO{
		DocumentID: job.Args.DocumentID,
	}); err != nil {
		return fmt.Errorf("failed to enqueue chunk translation: %w", err)
	}
	return nil
}

// indexOfChunk returns the index of the chunk with the given ID in chunks,
// or -1 if not found.
func indexOfChunk(chunks []model.DocumentChunk, id uuid.UUID) int {
	for i := range chunks {
		if chunks[i].ID == id {
			return i
		}
	}
	return -1
}

// gatherSurroundingText collects nearby body text/caption chunks (within
// +/-surroundingPageWindow pages of target) to ground a VLM caption request.
//
// Windows by Page, not SequenceNumber adjacency: mapDoclingDocument
// concatenates all texts, then all tables, then all pictures (a documented
// simplification, not true reading order), so a chunk's sequence-neighbors
// are typically other images/tables, not nearby body text — Page is the
// reliable signal instead. Docling's own LayoutTypeCaption chunks (if any)
// are gathered first, since they're the strongest available signal — this
// way they survive the character budget even if they sit late in `all`'s
// concatenated order.
func gatherSurroundingText(all []model.DocumentChunk, target *model.DocumentChunk) string {
	inWindow := func(c *model.DocumentChunk) bool {
		return c.ID != target.ID && contextualLayoutTypes[c.LayoutType] && c.Text != "" &&
			c.Page >= target.Page-surroundingPageWindow && c.Page <= target.Page+surroundingPageWindow
	}

	var b strings.Builder
	write := func(text string) bool {
		if b.Len()+len(text)+1 > surroundingTextBudget {
			return false
		}
		b.WriteString(text)
		b.WriteString("\n")
		return true
	}

	for i := range all {
		c := &all[i]
		if inWindow(c) && c.LayoutType == model.LayoutTypeCaption {
			if !write(c.Text) {
				return strings.TrimSpace(b.String())
			}
		}
	}
	for i := range all {
		c := &all[i]
		if inWindow(c) && c.LayoutType != model.LayoutTypeCaption {
			if !write(c.Text) {
				break
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// contentTypeFromKey recovers an image's content type from its S3 key's extension.
func contentTypeFromKey(key string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(key)), ".")
	return extensionContentTypes[ext]
}
