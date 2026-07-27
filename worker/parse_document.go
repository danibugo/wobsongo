package worker

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// parseDocumentTimeout bounds how long the worker waits for Docling to parse
// a document. Docling can take minutes on large PDFs — cold-starting a
// serverless Docling instance alone can take several minutes before
// conversion even begins — so this must stay comfortably above
// external.DoclingClient's own HTTP timeout (currently 9 minutes), leaving
// margin for this job's own S3 store + enqueue steps afterward. Blocking a
// River worker goroutine for that long is cheap in Go.
const parseDocumentTimeout = 11 * time.Minute

// rawOutputContentType is the content type stored alongside a document's
// raw Docling response in S3.
const rawOutputContentType = "application/json"

// ParseDocumentWorker is a River worker that fetches a document's raw
// Docling Serve output and hands it off for processing. Deliberately does
// nothing else: Docling responses can run to hundreds of megabytes
// (embedded images), and re-fetching from an external, rate-limited service
// on every retry is wasteful. This job's only responsibilities are to fetch
// once, persist the raw response durably (S3, not just this job's RAM), and
// enqueue ProcessParsedDocumentWorker — which does the actual chunk
// filtering/storage/image-extraction, and can be retried independently
// without ever calling Docling again.
type ParseDocumentWorker struct {
	// Embedding River's default worker behavior for the specific DTO.
	river.WorkerDefaults[queue.ParseDocumentDTO]
	// Processor calls out to the external document-parsing service (Docling).
	Processor data.DocumentProcessor
	// MediaService presigns the document's file key into a URL Docling can fetch.
	MediaService *service.MediaService
	// RawStore persists Docling's raw response so ProcessParsedDocumentWorker
	// can read it back without re-calling Docling.
	RawStore data.RawObjectStore
	// Enqueuer schedules ProcessParsedDocumentWorker's job.
	Enqueuer queue.JobEnqueuer
}

// NewParseDocumentWorker is a constructor for ParseDocumentWorker.
func NewParseDocumentWorker(
	processor data.DocumentProcessor,
	mediaService *service.MediaService,
	rawStore data.RawObjectStore,
	enqueuer queue.JobEnqueuer,
) *ParseDocumentWorker {
	return &ParseDocumentWorker{
		Processor:    processor,
		MediaService: mediaService,
		RawStore:     rawStore,
		Enqueuer:     enqueuer,
	}
}

// Timeout overrides River's default job timeout to accommodate slow Docling parses.
func (w *ParseDocumentWorker) Timeout(*river.Job[queue.ParseDocumentDTO]) time.Duration {
	return parseDocumentTimeout
}

// Work is the main method that gets called when a job is dequeued.
func (w *ParseDocumentWorker) Work(
	ctx context.Context,
	job *river.Job[queue.ParseDocumentDTO],
) error {
	// ttl=0 lets MediaService apply its own default (900s) — plenty, since
	// Docling fetches the file once up front, not throughout the whole parse.
	documentURL, err := w.MediaService.GetPresignedGETURL(ctx, job.Args.FileKey, 0)
	if err != nil {
		return fmt.Errorf("failed to presign document %s for docling: %w", job.Args.DocumentID, err)
	}

	raw, err := w.Processor.FetchRawFromURL(ctx, documentURL)
	if err != nil {
		return fmt.Errorf("failed to fetch document %s via docling: %w", job.Args.DocumentID, err)
	}

	rawKey := rawOutputKey(job.Args.DocumentID.String())
	if err := w.RawStore.PutObject(
		ctx,
		rawKey,
		bytes.NewReader(raw),
		int64(len(raw)),
		rawOutputContentType,
	); err != nil {
		return fmt.Errorf(
			"failed to store raw docling output for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	if err := w.Enqueuer.Enqueue(ctx, queue.ProcessParsedDocumentDTO{
		DocumentID:   job.Args.DocumentID,
		RawOutputKey: rawKey,
	}); err != nil {
		return fmt.Errorf(
			"failed to enqueue processing job for document %s: %w",
			job.Args.DocumentID,
			err,
		)
	}

	return nil
}

// rawOutputKey returns the S3 key a document's raw Docling response is
// stored under. Deliberately not run through data.ObjectPrefixForIntent —
// this key is never presigned-GET'd by a client, only read back by
// ProcessParsedDocumentWorker via data.RawObjectStore.
func rawOutputKey(documentID string) string {
	return fmt.Sprintf("parsed_output/%s.json", documentID)
}
