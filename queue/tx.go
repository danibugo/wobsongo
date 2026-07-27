package queue

import "context"

// JobEnqueuer defines the interface for enqueuing background jobs.
// Repos that need to enqueue jobs (optionally within a DB transaction)
// implement this interface. See data.TxAware for transaction support.
type JobEnqueuer interface {
	// Enqueue adds a job with the specified payload to the queue.
	Enqueue(ctx context.Context, payload BackgroundJob) error
}
