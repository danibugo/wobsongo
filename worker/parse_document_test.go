package worker

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

// stubProcessor is a hand-rolled data.DocumentProcessor for testing the
// worker without a real Docling Serve instance.
type stubProcessor struct {
	raw []byte
	err error
}

func (s *stubProcessor) FetchRawFromURL(_ context.Context, _ string) ([]byte, error) {
	return s.raw, s.err
}

// stubMediaProvider is a hand-rolled data.MediaUploadProvider for testing
// ParseDocumentWorker via a real *service.MediaService without real S3/MinIO.
type stubMediaProvider struct {
	presignedURL string
	err          error
}

func (s *stubMediaProvider) GetPresignedPOSTURL(
	_ context.Context,
	_ data.MediaUploadIntent,
	_, _ string,
) (*url.URL, map[string]string, error) {
	panic("not used in this test")
}

func (s *stubMediaProvider) GetPresignedGETURL(
	_ context.Context,
	_ string,
	_ int64,
) (string, error) {
	return s.presignedURL, s.err
}

func (s *stubMediaProvider) GetPresignedGETURLs(
	_ context.Context,
	_ []string,
	_ int64,
) (map[string]string, error) {
	panic("not used in this test")
}

// stubRawStore is a hand-rolled data.RawObjectStore for testing without real S3/MinIO.
type stubRawStore struct {
	putObjectFunc func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	getObjectFunc func(ctx context.Context, key string) (io.ReadCloser, error)
}

func (s *stubRawStore) PutObject(
	ctx context.Context,
	key string,
	r io.Reader,
	size int64,
	contentType string,
) error {
	if s.putObjectFunc == nil {
		return nil
	}
	return s.putObjectFunc(ctx, key, r, size, contentType)
}

func (s *stubRawStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.getObjectFunc(ctx, key)
}

// stubEnqueuer is a hand-rolled queue.JobEnqueuer for testing without a real River client.
type stubEnqueuer struct {
	enqueueFunc func(ctx context.Context, payload queue.BackgroundJob) error
}

func (s *stubEnqueuer) Enqueue(ctx context.Context, payload queue.BackgroundJob) error {
	if s.enqueueFunc == nil {
		return nil
	}
	return s.enqueueFunc(ctx, payload)
}

func newTestJob(fileKey string) *river.Job[queue.ParseDocumentDTO] {
	return &river.Job[queue.ParseDocumentDTO]{
		Args: queue.ParseDocumentDTO{DocumentID: uuid.New(), FileKey: fileKey},
	}
}

func TestParseDocumentWorker_Work_Success(t *testing.T) {
	mediaService := service.NewMediaService(
		&stubMediaProvider{presignedURL: "https://example.com/doc.pdf"},
	)
	processor := &stubProcessor{raw: []byte(`{"status":"success"}`)}

	var putKey string
	var putBody []byte
	rawStore := &stubRawStore{
		putObjectFunc: func(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
			putKey = key
			putBody, _ = io.ReadAll(r)
			if contentType != rawOutputContentType {
				t.Errorf("expected content type %q, got %q", rawOutputContentType, contentType)
			}
			return nil
		},
	}

	var enqueued queue.BackgroundJob
	enqueuer := &stubEnqueuer{
		enqueueFunc: func(_ context.Context, payload queue.BackgroundJob) error {
			enqueued = payload
			return nil
		},
	}

	w := NewParseDocumentWorker(processor, mediaService, rawStore, enqueuer)
	job := newTestJob("documents/abc.pdf")

	if err := w.Work(t.Context(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(putBody) != `{"status":"success"}` {
		t.Errorf("expected the raw docling response to be stored verbatim, got %q", putBody)
	}
	expectedKey := rawOutputKey(job.Args.DocumentID.String())
	if putKey != expectedKey {
		t.Errorf("expected raw output key %q, got %q", expectedKey, putKey)
	}

	dto, ok := enqueued.(queue.ProcessParsedDocumentDTO)
	if !ok {
		t.Fatalf("expected a queue.ProcessParsedDocumentDTO to be enqueued, got %T", enqueued)
	}
	if dto.DocumentID != job.Args.DocumentID {
		t.Errorf("expected enqueued DocumentID %s, got %s", job.Args.DocumentID, dto.DocumentID)
	}
	if dto.RawOutputKey != expectedKey {
		t.Errorf("expected enqueued RawOutputKey %q, got %q", expectedKey, dto.RawOutputKey)
	}
}

func TestParseDocumentWorker_Work_PresignError(t *testing.T) {
	mediaService := service.NewMediaService(&stubMediaProvider{err: errors.New("boom")})
	w := NewParseDocumentWorker(&stubProcessor{}, mediaService, &stubRawStore{}, &stubEnqueuer{})

	if err := w.Work(t.Context(), newTestJob("documents/abc.pdf")); err == nil {
		t.Fatal("expected an error when presigning fails")
	}
}

func TestParseDocumentWorker_Work_FetchError(t *testing.T) {
	mediaService := service.NewMediaService(
		&stubMediaProvider{presignedURL: "https://example.com/doc.pdf"},
	)
	processor := &stubProcessor{err: errors.New("docling down")}
	w := NewParseDocumentWorker(processor, mediaService, &stubRawStore{}, &stubEnqueuer{})

	if err := w.Work(t.Context(), newTestJob("documents/abc.pdf")); err == nil {
		t.Fatal("expected an error when the processor fails")
	}
}

func TestParseDocumentWorker_Work_PutObjectError(t *testing.T) {
	mediaService := service.NewMediaService(
		&stubMediaProvider{presignedURL: "https://example.com/doc.pdf"},
	)
	processor := &stubProcessor{raw: []byte(`{}`)}
	rawStore := &stubRawStore{
		putObjectFunc: func(context.Context, string, io.Reader, int64, string) error {
			return errors.New("s3 down")
		},
	}
	enqueuer := &stubEnqueuer{
		enqueueFunc: func(context.Context, queue.BackgroundJob) error {
			t.Error("Enqueue should not be called when PutObject fails")
			return nil
		},
	}

	w := NewParseDocumentWorker(processor, mediaService, rawStore, enqueuer)
	if err := w.Work(t.Context(), newTestJob("documents/abc.pdf")); err == nil {
		t.Fatal("expected an error when PutObject fails")
	}
}

func TestParseDocumentWorker_Work_EnqueueError(t *testing.T) {
	mediaService := service.NewMediaService(
		&stubMediaProvider{presignedURL: "https://example.com/doc.pdf"},
	)
	processor := &stubProcessor{raw: []byte(`{}`)}
	rawStore := &stubRawStore{}
	enqueuer := &stubEnqueuer{
		enqueueFunc: func(context.Context, queue.BackgroundJob) error {
			return errors.New("queue down")
		},
	}

	w := NewParseDocumentWorker(processor, mediaService, rawStore, enqueuer)
	if err := w.Work(t.Context(), newTestJob("documents/abc.pdf")); err == nil {
		t.Fatal("expected an error when Enqueue fails")
	}
}
