package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"
)

// extractKnowledgeDefaultConcurrency is used when Concurrency is unset (<=0)
// — e.g. a caller constructing the worker directly (tests) without
// specifying it. config.ExtractionConfig applies this same default when
// EXTRACTION_CONCURRENCY isn't set, so the two stay in sync.
const extractKnowledgeDefaultConcurrency = 5

// extractKnowledgeFallbackTimeout is used only if the live pending-chunk
// count can't be determined (the sizing query in Timeout() itself fails) —
// generous enough to get through a handful of chunks safely rather than
// erroring out immediately.
const extractKnowledgeFallbackTimeout = 15 * time.Minute

// extractKnowledgeFixedOverhead covers the job's work outside the per-chunk
// loop: fetching the document and its chunk list up front, plus the final
// enqueue call.
const extractKnowledgeFixedOverhead = 1 * time.Minute

// extractKnowledgePerChunkBudget bounds how long a single round of
// concurrent chunks (see ExtractKnowledgeWorker.Concurrency) gets within the
// job's overall timeout — margin over ExtractionClient's own per-call HTTP timeout
// (7 minutes, external/extraction_client.go's extractionHTTPTimeout) to also
// cover that chunk's DB write.
const extractKnowledgePerChunkBudget = 8 * time.Minute

// extractKnowledgeBatchSize caps how many chunks a single job execution
// extracts before re-enqueueing a continuation job for the rest (see
// Work()). This is deliberate, not just a nice-to-have: River's JobRescuer
// considers any job "stuck" and force-retries it after a flat, client-wide
// duration (1 hour by default) that knows nothing about a worker's own
// Timeout() override — confirmed against a real job whose error history
// showed "Stuck job rescued by JobRescuer" partway through a large document,
// well before it had actually failed. Scaling Timeout() to cover an entire
// large backlog in one execution (the previous approach) only pushes the
// mismatch further: it would require inflating RescueStuckJobsAfter to many
// hours or days client-wide, which defeats its purpose of catching genuinely
// crashed processes quickly. Capping the batch keeps a single execution's
// Timeout() — and therefore the client-wide RescueStuckJobsAfter needed to
// stay safely above it — small and constant regardless of total document
// size; a huge document just takes more job hops, which is fine since
// progress commits per-chunk already. At the default concurrency (5) this
// caps Timeout() around 41 minutes (extractKnowledgeFixedOverhead +
// ceil(25/5)*extractKnowledgePerChunkBudget), comfortably under River's
// 1-hour default RescueStuckJobsAfter.
const extractKnowledgeBatchSize = 25

// ExtractKnowledgeWorker is a River worker that extracts atomic knowledge
// facts (subject-predicate-object, with a truth-tier classification) from a
// document's text-bearing, not-yet-extracted chunks. Kept as its own job
// (not inline in chunk storage/captioning) so a slow/costly/rate-limited LLM
// call never fails or retries those steps themselves.
type ExtractKnowledgeWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.ExtractKnowledgeDTO]
	// ChunkRepo lists chunks needing extraction.
	ChunkRepo data.DocumentChunkRepoer
	// KnowledgeRepo stores extracted facts and marks chunks as processed.
	KnowledgeRepo data.AtomicKnowledgeRepoer
	// DocumentService fetches the parent document's title, for grounding.
	DocumentService *service.DocumentService
	// Extractor extracts facts from a chunk's text.
	Extractor data.KnowledgeExtractor
	// Concurrency bounds how many chunks are extracted at once. <=0 falls
	// back to extractKnowledgeDefaultConcurrency — see concurrency().
	Concurrency int
}

// NewExtractKnowledgeWorker is a constructor for ExtractKnowledgeWorker.
// concurrency <=0 falls back to extractKnowledgeDefaultConcurrency.
func NewExtractKnowledgeWorker(
	chunkRepo data.DocumentChunkRepoer,
	knowledgeRepo data.AtomicKnowledgeRepoer,
	documentService *service.DocumentService,
	extractor data.KnowledgeExtractor,
	concurrency int,
) *ExtractKnowledgeWorker {
	return &ExtractKnowledgeWorker{
		ChunkRepo:       chunkRepo,
		KnowledgeRepo:   knowledgeRepo,
		DocumentService: documentService,
		Extractor:       extractor,
		Concurrency:     concurrency,
	}
}

// concurrency resolves the effective concurrency limit, applying
// extractKnowledgeDefaultConcurrency if Concurrency is unset or invalid.
func (w *ExtractKnowledgeWorker) concurrency() int {
	if w.Concurrency <= 0 {
		return extractKnowledgeDefaultConcurrency
	}
	return w.Concurrency
}

// Timeout overrides River's default job timeout, scaled to the number of
// chunks this execution will actually attempt (bounded by
// extractKnowledgeBatchSize, never the full remaining backlog — see that
// constant's comment for why). River evaluates this fresh on every attempt
// (right before setting that attempt's deadline).
func (w *ExtractKnowledgeWorker) Timeout(job *river.Job[queue.ExtractKnowledgeDTO]) time.Duration {
	chunks, err := w.ChunkRepo.ListChunksNeedingKnowledgeExtraction(
		context.Background(),
		job.Args.DocumentID,
	)
	if err != nil {
		return extractKnowledgeFallbackTimeout
	}
	batchLen := min(len(chunks), extractKnowledgeBatchSize)
	rounds := (batchLen + w.concurrency() - 1) / w.concurrency()
	return extractKnowledgeFixedOverhead + time.Duration(rounds)*extractKnowledgePerChunkBudget
}

// Work is the main method that gets called when a job is dequeued.
func (w *ExtractKnowledgeWorker) Work(
	ctx context.Context,
	job *river.Job[queue.ExtractKnowledgeDTO],
) error {
	doc, err := w.DocumentService.GetByID(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("failed to fetch document %s: %w", job.Args.DocumentID, err)
	}

	chunks, err := w.ChunkRepo.ListChunksNeedingKnowledgeExtraction(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to list chunks needing knowledge extraction for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	// Cap this execution to extractKnowledgeBatchSize chunks — see that
	// constant's comment. Whatever's left gets a continuation job below,
	// rather than trying to finish an arbitrarily large backlog in one go.
	moreRemaining := len(chunks) > extractKnowledgeBatchSize
	batch := chunks
	if moreRemaining {
		batch = chunks[:extractKnowledgeBatchSize]
	}

	// Bounded concurrency, not errgroup.WithContext: a plain group means one
	// chunk's failure doesn't cancel its concurrently in-flight siblings —
	// each chunk commits independently (see extractChunk), so there's no
	// reason to throw away a sibling's still-succeeding LLM call just
	// because another chunk errored. g.Wait() below still surfaces the
	// first error so Work() fails and River retries the (now smaller)
	// remaining set.
	var g errgroup.Group
	g.SetLimit(w.concurrency())
	for i := range batch {
		chunk := &batch[i]
		g.Go(func() error {
			return w.extractChunk(ctx, doc, chunk)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	if moreRemaining {
		// More chunks remain beyond this batch — re-enqueue for the rest
		// rather than assuming this document is fully processed.
		if err := w.ChunkRepo.Enqueue(ctx, queue.ExtractKnowledgeDTO{
			DocumentID: job.Args.DocumentID,
		}); err != nil {
			return fmt.Errorf("failed to enqueue extraction continuation: %w", err)
		}
		return nil
	}

	// Every text-bearing chunk in this document has now had extraction run
	// (successfully, even if some yielded zero facts) — safe to embed.
	if err := w.ChunkRepo.Enqueue(ctx, queue.EmbedKnowledgeDTO{
		DocumentID: job.Args.DocumentID,
	}); err != nil {
		return fmt.Errorf("failed to enqueue knowledge embedding: %w", err)
	}
	return nil
}

// extractChunk extracts and stores facts for a single chunk, then marks it
// extracted. Safe to call concurrently across chunks: each call opens its
// own transaction (data.AtomicKnowledgeRepoer.WithTx acquires a fresh
// connection per call) and touches only its own chunk.
func (w *ExtractKnowledgeWorker) extractChunk(
	ctx context.Context,
	doc *model.Document,
	chunk *model.DocumentChunk,
) error {
	extracted, err := w.Extractor.Extract(ctx, &data.ExtractionRequest{
		Text:            chunk.Text,
		DocumentTitle:   doc.Title,
		PublisherName:   doc.PublisherName,
		PublicationYear: doc.PublicationYear,
		Language:        doc.Language,
	})
	if err != nil {
		return fmt.Errorf("failed to extract knowledge for chunk %s: %w", chunk.ID, err)
	}

	now := time.Now()
	facts := make([]model.AtomicKnowledge, 0, len(extracted))
	for j := range extracted {
		if extracted[j].Category == model.FactCategoryMetadata {
			// Never persisted — administrative/bibliographic content about
			// the document itself, not clinical content. Logged (not
			// silently dropped) as a cheap audit trail against
			// misclassification: FactCategoryUnknown is still stored, so
			// only confidently-metadata facts are discarded here.
			log.Printf(
				"[ExtractKnowledgeWorker] discarding metadata-category fact: subject=%q predicate=%q object=%q chunk=%s",
				extracted[j].Subject,
				extracted[j].Predicate,
				extracted[j].Object,
				chunk.ID,
			)
			continue
		}

		topics := extracted[j].Topics
		if topics == nil {
			// Must be non-nil: this batch insert goes through Postgres's
			// COPY protocol (sqlc :copyfrom), which sends every column's
			// literal value — a nil slice encodes as SQL NULL over COPY
			// and violates topics' NOT NULL constraint, since COPY never
			// falls back to a column's DEFAULT the way a plain INSERT
			// would. extracted[j].Topics comes from LLM JSON output,
			// which can omit or null the field.
			topics = []string{}
		}
		facts = append(facts, model.AtomicKnowledge{
			ID:                   uuid.New(),
			CreatedAt:            now,
			UpdatedAt:            now,
			DocumentID:           doc.ID,
			DocumentChunkID:      chunk.ID,
			TruthTier:            extracted[j].TruthTier,
			Category:             extracted[j].Category,
			Topics:               topics,
			Subject:              extracted[j].Subject,
			Predicate:            extracted[j].Predicate,
			Object:               extracted[j].Object,
			Note:                 extracted[j].Note,
			Language:             doc.Language,
			SearchTextTranslated: extracted[j].TranslatedSearchText,
		})
	}

	err = w.KnowledgeRepo.WithTx(ctx, func(tx data.AtomicKnowledgeRepoer) error {
		if err := tx.CreateBatch(ctx, facts); err != nil {
			return fmt.Errorf("failed to store extracted facts: %w", err)
		}
		return tx.MarkChunkKnowledgeExtracted(ctx, chunk.ID)
	})
	if err != nil {
		return fmt.Errorf("failed to commit extraction results for chunk %s: %w", chunk.ID, err)
	}
	return nil
}
