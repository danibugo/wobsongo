package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"
)

// translateChunksDefaultConcurrency is used when Concurrency is unset (<=0)
// — e.g. a caller constructing the worker directly (tests) without
// specifying it. config.TranslationConfig applies this same default when
// TRANSLATION_CONCURRENCY isn't set, so the two stay in sync.
const translateChunksDefaultConcurrency = 5

// translateChunksFallbackTimeout is used only if the live pending-chunk
// count can't be determined (the sizing query in Timeout() itself fails) —
// generous enough to get through a handful of chunks safely rather than
// erroring out immediately.
const translateChunksFallbackTimeout = 15 * time.Minute

// translateChunksFixedOverhead covers the job's work outside the per-chunk
// loop: fetching the pending chunk list up front, plus the final enqueue call.
const translateChunksFixedOverhead = 1 * time.Minute

// translateChunksPerChunkBudget bounds how long a single round of concurrent
// chunks (see TranslateChunksWorker.Concurrency) gets within the job's
// overall timeout — margin over TranslationClient's own per-call HTTP
// timeout (7 minutes, external/translation_client.go's translationHTTPTimeout)
// to also cover that chunk's DB write.
const translateChunksPerChunkBudget = 8 * time.Minute

// translateChunksBatchSize caps how many chunks a single job execution
// translates before re-enqueueing a continuation job for the rest — same
// reasoning as extractKnowledgeBatchSize (internal/worker/extract_knowledge.go):
// keeps a single execution's Timeout() safely under River's default
// RescueStuckJobsAfter regardless of total document size.
const translateChunksBatchSize = 25

// TranslateChunksWorker is a River worker that translates a document's
// text-bearing, not-yet-translated chunks into the other supported language,
// so full-text search can find them regardless of query language. Kept as
// its own job (not inline in chunk storage), mirroring ExtractKnowledgeWorker:
// a slow/costly/rate-limited LLM call never fails or retries other steps.
type TranslateChunksWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.TranslateChunksDTO]
	// ChunkRepo lists chunks needing translation and persists results.
	ChunkRepo data.DocumentChunkRepoer
	// DocumentService fetches the parent document, for its Language.
	DocumentService *service.DocumentService
	// Translator translates a chunk's text into the other supported language.
	Translator data.Translator
	// Concurrency bounds how many chunks are translated at once. <=0 falls
	// back to translateChunksDefaultConcurrency — see concurrency().
	Concurrency int
}

// NewTranslateChunksWorker is a constructor for TranslateChunksWorker.
// concurrency <=0 falls back to translateChunksDefaultConcurrency.
func NewTranslateChunksWorker(
	chunkRepo data.DocumentChunkRepoer,
	documentService *service.DocumentService,
	translator data.Translator,
	concurrency int,
) *TranslateChunksWorker {
	return &TranslateChunksWorker{
		ChunkRepo:       chunkRepo,
		DocumentService: documentService,
		Translator:      translator,
		Concurrency:     concurrency,
	}
}

// concurrency resolves the effective concurrency limit, applying
// translateChunksDefaultConcurrency if Concurrency is unset or invalid.
func (w *TranslateChunksWorker) concurrency() int {
	if w.Concurrency <= 0 {
		return translateChunksDefaultConcurrency
	}
	return w.Concurrency
}

// Timeout overrides River's default job timeout, scaled to the number of
// chunks this execution will actually attempt (bounded by
// translateChunksBatchSize, never the full remaining backlog — see that
// constant's comment for why). River evaluates this fresh on every attempt
// (right before setting that attempt's deadline).
func (w *TranslateChunksWorker) Timeout(job *river.Job[queue.TranslateChunksDTO]) time.Duration {
	chunks, err := w.ChunkRepo.ListChunksNeedingTranslation(
		context.Background(),
		job.Args.DocumentID,
	)
	if err != nil {
		return translateChunksFallbackTimeout
	}
	batchLen := min(len(chunks), translateChunksBatchSize)
	rounds := (batchLen + w.concurrency() - 1) / w.concurrency()
	return translateChunksFixedOverhead + time.Duration(rounds)*translateChunksPerChunkBudget
}

// Work is the main method that gets called when a job is dequeued.
func (w *TranslateChunksWorker) Work(
	ctx context.Context,
	job *river.Job[queue.TranslateChunksDTO],
) error {
	doc, err := w.DocumentService.GetByID(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("failed to fetch document %s: %w", job.Args.DocumentID, err)
	}

	chunks, err := w.ChunkRepo.ListChunksNeedingTranslation(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to list chunks needing translation for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	// Cap this execution to translateChunksBatchSize chunks — see that
	// constant's comment. Whatever's left gets a continuation job below,
	// rather than trying to finish an arbitrarily large backlog in one go.
	moreRemaining := len(chunks) > translateChunksBatchSize
	batch := chunks
	if moreRemaining {
		batch = chunks[:translateChunksBatchSize]
	}

	// Bounded concurrency, not errgroup.WithContext: a plain group means one
	// chunk's failure doesn't cancel its concurrently in-flight siblings —
	// each chunk commits independently, so there's no reason to throw away a
	// sibling's still-succeeding LLM call just because another chunk
	// errored. g.Wait() below still surfaces the first error so Work() fails
	// and River retries the (now smaller) remaining set.
	var g errgroup.Group
	g.SetLimit(w.concurrency())
	for i := range batch {
		chunk := &batch[i]
		g.Go(func() error {
			translated, err := w.Translator.Translate(ctx, chunk.Text, doc.Language)
			if err != nil {
				return fmt.Errorf("failed to translate chunk %s: %w", chunk.ID, err)
			}
			if err := w.ChunkRepo.UpdateChunkTranslation(ctx, chunk.ID, translated); err != nil {
				return fmt.Errorf("failed to store translation for chunk %s: %w", chunk.ID, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	if moreRemaining {
		// More chunks remain beyond this batch — re-enqueue for the rest
		// rather than assuming this document is fully translated.
		if err := w.ChunkRepo.Enqueue(ctx, queue.TranslateChunksDTO{
			DocumentID: job.Args.DocumentID,
		}); err != nil {
			return fmt.Errorf("failed to enqueue translation continuation: %w", err)
		}
	}
	return nil
}
