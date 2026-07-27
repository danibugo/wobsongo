package data

import (
	"context"

	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
)

// DocumentRepoer defines the interface for interacting with document data storage.
type DocumentRepoer interface {
	CrudableWithTx[model.Document, DocumentRepoer]
	queue.JobEnqueuer

	// GetBySHA256 retrieves a document by its content hash. Returns
	// ErrNotFound if no document has that hash.
	GetBySHA256(ctx context.Context, sha256 string) (*model.Document, error)
}
