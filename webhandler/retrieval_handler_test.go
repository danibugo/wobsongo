package webhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newRetrievalTestApp builds an *echo.Echo wired with a DocumentRepo,
// DocumentChunkRepo, AtomicKnowledgeRepo, MediaProvider, and Embedder, for
// retrieval_handler tests.
func newRetrievalTestApp(
	documentRepo *mockrepo.DocumentRepoerMock,
	chunkRepo *mockrepo.DocumentChunkRepoerMock,
	knowledgeRepo *mockrepo.AtomicKnowledgeRepoerMock,
	mediaProvider data.MediaUploadProvider,
	embedder data.Embedder,
) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := &webhandler.WebRepos{
		DocumentRepo:  documentRepo,
		ChunkRepo:     chunkRepo,
		KnowledgeRepo: knowledgeRepo,
		MediaProvider: mediaProvider,
		Embedder:      embedder,
	}
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func TestRetrievalListPage(t *testing.T) {
	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		e, _ := newRetrievalTestApp(&mockrepo.DocumentRepoerMock{}, chunkRepo, knowledgeRepo, &stubMediaProvider{}, &stubEmbedder{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/retrieval", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("empty query renders enter-a-query state without searching", func(t *testing.T) {
		// Every dependency is left as a panicking/nil stub: if Search is
		// invoked at all, the test panics, proving the empty-query
		// short-circuit.
		e, jwtAuth := newRetrievalTestApp(
			&mockrepo.DocumentRepoerMock{}, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{},
			&panicIfGETCalledMediaProvider{}, &stubEmbedder{err: context.DeadlineExceeded},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Enter a query to search the knowledge base.") {
			t.Errorf("expected enter-a-query message, got body: %s", rec.Body.String())
		}
	})

	t.Run("non-empty query with no results renders no-results state", func(t *testing.T) {
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		e, jwtAuth := newRetrievalTestApp(
			&mockrepo.DocumentRepoerMock{}, chunkRepo, knowledgeRepo,
			&stubMediaProvider{}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=cancer+cure", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		// %q formatting is HTML-escaped by templ: quotes render as &#34;.
		if !strings.Contains(rec.Body.String(), "No results for &#34;cancer cure&#34;.") {
			t.Errorf("expected no-results message, got body: %s", rec.Body.String())
		}
	})

	t.Run("embedder error propagates raw (500)", func(t *testing.T) {
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		e, jwtAuth := newRetrievalTestApp(
			&mockrepo.DocumentRepoerMock{}, chunkRepo, knowledgeRepo,
			&stubMediaProvider{}, &stubEmbedder{err: data.ErrInternal},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=cancer", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("chunk hit resolves document title and PDF URL", func(t *testing.T) {
		docID := uuid.New()
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		chunkRepo.SearchByEmbeddingFunc = func(
			context.Context, []float32, int,
		) ([]data.ScoredResult[model.DocumentChunk], error) {
			return []data.ScoredResult[model.DocumentChunk]{
				{Item: model.DocumentChunk{
					ID: uuid.New(), DocumentID: docID,
					ParsedChunk: model.ParsedChunk{Text: "hybrid search hit text"},
				}, Score: 0.9},
			}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: docID, Title: "Found Doc", Filetype: "application/pdf", FileURL: "documents/x.pdf"}, nil
		}
		e, jwtAuth := newRetrievalTestApp(
			docRepo, chunkRepo, knowledgeRepo,
			&stubMediaProvider{presignedURL: "https://example.com/rag-pdf"}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=hybrid", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "hybrid search hit text") {
			t.Errorf("expected chunk text in body, got: %s", body)
		}
		if !strings.Contains(body, "Found Doc") {
			t.Errorf("expected document title in body, got: %s", body)
		}
		if !strings.Contains(body, "https://example.com/rag-pdf") {
			t.Errorf("expected presigned URL in body, got: %s", body)
		}
	})

	t.Run("resolveDoc caches by DocumentID across multiple hits", func(t *testing.T) {
		docID := uuid.New()
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		chunkRepo.SearchByEmbeddingFunc = func(
			context.Context, []float32, int,
		) ([]data.ScoredResult[model.DocumentChunk], error) {
			return []data.ScoredResult[model.DocumentChunk]{
				{Item: model.DocumentChunk{ID: uuid.New(), DocumentID: docID, ParsedChunk: model.ParsedChunk{Text: "hit one"}}},
				{Item: model.DocumentChunk{ID: uuid.New(), DocumentID: docID, ParsedChunk: model.ParsedChunk{Text: "hit two"}}},
			}, nil
		}
		var callCount atomic.Int32
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			callCount.Add(1)
			return &model.Document{ID: docID, Title: "Shared Doc"}, nil
		}
		e, jwtAuth := newRetrievalTestApp(
			docRepo, chunkRepo, knowledgeRepo, &stubMediaProvider{}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=shared", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := callCount.Load(); got != 1 {
			t.Errorf("expected GetByID called once (cached), got %d calls", got)
		}
	})

	t.Run("resolveDoc failure aborts the whole page (500), even though Search succeeded", func(t *testing.T) {
		docID := uuid.New()
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		chunkRepo.SearchByEmbeddingFunc = func(
			context.Context, []float32, int,
		) ([]data.ScoredResult[model.DocumentChunk], error) {
			return []data.ScoredResult[model.DocumentChunk]{
				{Item: model.DocumentChunk{ID: uuid.New(), DocumentID: docID}},
			}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		e, jwtAuth := newRetrievalTestApp(
			docRepo, chunkRepo, knowledgeRepo, &stubMediaProvider{}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=broken", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 (resolveDoc failure aborts the page), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("presign error for a PDF result document propagates raw (500)", func(t *testing.T) {
		docID := uuid.New()
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		chunkRepo.SearchByEmbeddingFunc = func(
			context.Context, []float32, int,
		) ([]data.ScoredResult[model.DocumentChunk], error) {
			return []data.ScoredResult[model.DocumentChunk]{
				{Item: model.DocumentChunk{ID: uuid.New(), DocumentID: docID, ParsedChunk: model.ParsedChunk{Text: "pdf hit"}}},
			}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: docID, Filetype: "application/pdf", FileURL: "documents/x.pdf"}, nil
		}
		e, jwtAuth := newRetrievalTestApp(
			docRepo, chunkRepo, knowledgeRepo, &stubMediaProvider{err: data.ErrInternal}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=pdf", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("non-PDF result document never calls media provider", func(t *testing.T) {
		docID := uuid.New()
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		chunkRepo.SearchByEmbeddingFunc = func(
			context.Context, []float32, int,
		) ([]data.ScoredResult[model.DocumentChunk], error) {
			return []data.ScoredResult[model.DocumentChunk]{
				{Item: model.DocumentChunk{ID: uuid.New(), DocumentID: docID, ParsedChunk: model.ParsedChunk{Text: "text hit"}}},
			}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: docID, Title: "Non-PDF Doc", Filetype: "text/markdown"}, nil
		}
		e, jwtAuth := newRetrievalTestApp(
			docRepo, chunkRepo, knowledgeRepo, &panicIfGETCalledMediaProvider{}, &stubEmbedder{vector: []float32{0.1}},
		)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/retrieval?q=text", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Non-PDF Doc") {
			t.Errorf("expected document title in body, got: %s", rec.Body.String())
		}
	})
}
