package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/riverqueue/river"
)

// embedKnowledgeTimeout bounds how long the worker waits across all of a
// document's facts. Generous relative to a single embedding call since a
// document can carry many facts, embedded in batches.
const embedKnowledgeTimeout = 10 * time.Minute

// EmbedKnowledgeWorker is a River worker that computes and stores embeddings
// for a document's not-yet-embedded atomic knowledge facts. Kept as its own
// job (not inline in extraction) so a slow/costly/rate-limited embedding
// call never fails or retries fact extraction itself.
type EmbedKnowledgeWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.EmbedKnowledgeDTO]
	// KnowledgeRepo reads facts needing embedding and saves their vectors.
	KnowledgeRepo data.AtomicKnowledgeRepoer
	// Embedder computes the embedding vector for a batch of fact texts.
	Embedder data.Embedder
}

// NewEmbedKnowledgeWorker is a constructor for EmbedKnowledgeWorker.
func NewEmbedKnowledgeWorker(
	knowledgeRepo data.AtomicKnowledgeRepoer,
	embedder data.Embedder,
) *EmbedKnowledgeWorker {
	return &EmbedKnowledgeWorker{
		KnowledgeRepo: knowledgeRepo,
		Embedder:      embedder,
	}
}

// Timeout overrides River's default job timeout.
func (w *EmbedKnowledgeWorker) Timeout(*river.Job[queue.EmbedKnowledgeDTO]) time.Duration {
	return embedKnowledgeTimeout
}

// Work is the main method that gets called when a job is dequeued.
func (w *EmbedKnowledgeWorker) Work(
	ctx context.Context,
	job *river.Job[queue.EmbedKnowledgeDTO],
) error {
	facts, err := w.KnowledgeRepo.ListNeedingEmbedding(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to list facts needing embedding for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}
	if len(facts) == 0 {
		// Nothing to do — already fully embedded (e.g. a retry after a
		// prior attempt got partway through).
		return nil
	}

	for start := 0; start < len(facts); start += embedBatchSize {
		end := min(start+embedBatchSize, len(facts))
		batch := facts[start:end]

		texts := make([]string, len(batch))
		for i := range batch {
			texts[i] = batch[i].SPOText()
		}

		vectors, err := w.Embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf(
				"failed to embed facts %d-%d for document %s: %w",
				start, end, job.Args.DocumentID, err,
			)
		}
		if len(vectors) != len(batch) {
			return fmt.Errorf(
				"embedder returned %d vectors for %d facts in document %s",
				len(vectors), len(batch), job.Args.DocumentID,
			)
		}

		for i := range batch {
			if err := w.KnowledgeRepo.UpdateEmbedding(ctx, batch[i].ID, vectors[i]); err != nil {
				return fmt.Errorf(
					"failed to save embedding for fact %s: %w",
					batch[i].ID,
					err,
				)
			}
		}
	}
	return nil
}
