package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/riverqueue/river"
)

// embedChunksTimeout bounds how long the worker waits across all of a
// document's chunks. Generous relative to a single embedding call since a
// document can carry many chunks, embedded in batches.
const embedChunksTimeout = 10 * time.Minute

// embedBatchSize caps how many chunk texts are sent to the Embedder per
// call. A conservative, hardcoded default (not env-configurable) — self-
// hosted OpenAI-compatible servers often cap batch size well below OpenAI's
// own 2048-input limit, and there's no concrete provider target yet to size
// this against precisely.
const embedBatchSize = 96

// EmbedChunksWorker is a River worker that computes and stores embeddings
// for a document's text-bearing, not-yet-embedded chunks. Kept as its own
// job (not inline in ProcessParsedDocumentWorker/CaptionImageChunksWorker) so
// a slow/costly/rate-limited embedding call never fails or retries chunk
// storage or captioning itself.
type EmbedChunksWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.EmbedChunksDTO]
	// ChunkRepo reads chunks needing embedding and saves their vectors.
	ChunkRepo data.DocumentChunkRepoer
	// Embedder computes the embedding vector for a batch of chunk texts.
	Embedder data.Embedder
}

// NewEmbedChunksWorker is a constructor for EmbedChunksWorker.
func NewEmbedChunksWorker(
	chunkRepo data.DocumentChunkRepoer,
	embedder data.Embedder,
) *EmbedChunksWorker {
	return &EmbedChunksWorker{
		ChunkRepo: chunkRepo,
		Embedder:  embedder,
	}
}

// Timeout overrides River's default job timeout.
func (w *EmbedChunksWorker) Timeout(*river.Job[queue.EmbedChunksDTO]) time.Duration {
	return embedChunksTimeout
}

// Work is the main method that gets called when a job is dequeued.
func (w *EmbedChunksWorker) Work(ctx context.Context, job *river.Job[queue.EmbedChunksDTO]) error {
	chunks, err := w.ChunkRepo.ListChunksNeedingEmbedding(ctx, job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf(
			"failed to list chunks needing embedding for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}
	if len(chunks) == 0 {
		// Nothing to do — already fully embedded (e.g. a retry after a
		// prior attempt got partway through), or no text-bearing chunks.
		return nil
	}

	for start := 0; start < len(chunks); start += embedBatchSize {
		end := min(start+embedBatchSize, len(chunks))
		batch := chunks[start:end]

		texts := make([]string, len(batch))
		for i := range batch {
			texts[i] = batch[i].Text
		}

		vectors, err := w.Embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf(
				"failed to embed chunks %d-%d for document %s: %w",
				start, end, job.Args.DocumentID, err,
			)
		}
		if len(vectors) != len(batch) {
			return fmt.Errorf(
				"embedder returned %d vectors for %d chunks in document %s",
				len(vectors), len(batch), job.Args.DocumentID,
			)
		}

		for i := range batch {
			batch[i].Embedding = vectors[i]
			batch[i].UpdatedAt = time.Now()
			if err := w.ChunkRepo.Update(ctx, &batch[i]); err != nil {
				return fmt.Errorf(
					"failed to save embedding for chunk %s: %w",
					batch[i].ID,
					err,
				)
			}
		}
	}
	return nil
}
