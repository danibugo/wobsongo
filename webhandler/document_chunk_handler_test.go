package webhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newChunkTestApp builds an *echo.Echo wired with a DocumentRepo,
// DocumentChunkRepo, and MediaProvider, for document_chunk_handler tests.
func newChunkTestApp(
	documentRepo *mockrepo.DocumentRepoerMock,
	chunkRepo *mockrepo.DocumentChunkRepoerMock,
	mediaProvider data.MediaUploadProvider,
) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := &webhandler.WebRepos{
		DocumentRepo:  documentRepo,
		ChunkRepo:     chunkRepo,
		MediaProvider: mediaProvider,
	}
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func TestDocumentChunkListPage(t *testing.T) {
	emptyDocRepo := func() *mockrepo.DocumentRepoerMock {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return &dto.PaginationResults[model.Document]{}, nil
		}
		return repo
	}

	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newChunkTestApp(emptyDocRepo(), &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/chunks", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("no document_id renders empty state", func(t *testing.T) {
		e, jwtAuth := newChunkTestApp(emptyDocRepo(), &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Select a document to view its chunks.") {
			t.Errorf("expected empty-state message, got body: %s", rec.Body.String())
		}
	})

	t.Run("invalid document_id renders same empty state", func(t *testing.T) {
		e, jwtAuth := newChunkTestApp(emptyDocRepo(), &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id=not-a-uuid", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Select a document to view its chunks.") {
			t.Errorf("expected empty-state message, got body: %s", rec.Body.String())
		}
	})

	t.Run("document not found renders same empty state (swallowed)", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Select a document to view its chunks.") {
			t.Errorf("expected empty-state message, got body: %s", rec.Body.String())
		}
	})

	t.Run("other document lookup error propagates raw (500)", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return nil, data.ErrInternal
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("document with zero pages renders no-chunks-yet state", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 0}, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "No chunks for this document yet.") {
			t.Errorf("expected no-chunks-yet message, got body: %s", rec.Body.String())
		}
	})

	t.Run("document with pages but no chunks on page renders empty page state", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 3}, nil
		}
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListByDocumentIDAndPageFunc = func(context.Context, uuid.UUID, int) ([]model.DocumentChunk, error) {
			return nil, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "No chunks detected on this page.") {
			t.Errorf("expected no-chunks-on-page message, got body: %s", rec.Body.String())
		}
	})

	t.Run("chunks present render table", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 1}, nil
		}
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListByDocumentIDAndPageFunc = func(context.Context, uuid.UUID, int) ([]model.DocumentChunk, error) {
			return []model.DocumentChunk{
				{ID: uuid.New(), ParsedChunk: model.ParsedChunk{Text: "hello chunk text"}},
			}, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "hello chunk text") {
			t.Errorf("expected chunk text in body, got: %s", rec.Body.String())
		}
	})

	t.Run("page beyond total pages clamps to last page", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 3}, nil
		}
		var capturedPage int
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListByDocumentIDAndPageFunc = func(_ context.Context, _ uuid.UUID, page int) ([]model.DocumentChunk, error) {
			capturedPage = page
			return nil, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String()+"&page=999", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedPage != 3 {
			t.Errorf("expected page clamped to 3, got %d", capturedPage)
		}
	})

	t.Run("ListByPage error propagates raw (500)", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 1}, nil
		}
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.ListByDocumentIDAndPageFunc = func(context.Context, uuid.UUID, int) ([]model.DocumentChunk, error) {
			return nil, data.ErrInternal
		}
		e, jwtAuth := newChunkTestApp(docRepo, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("PDF document fetches presigned URL", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 0, Filetype: "application/pdf", FileURL: "documents/x.pdf"}, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{
			presignedURL: "https://example.com/presigned-pdf-url",
		})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "https://example.com/presigned-pdf-url") {
			t.Errorf("expected presigned URL in body, got: %s", rec.Body.String())
		}
	})

	t.Run("non-PDF document never calls media provider", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 0, Filetype: "text/markdown"}, nil
		}
		// panicIfGETCalledMediaProvider panics if GetPresignedGETURL is
		// called at all, proving the non-PDF branch skips it entirely.
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &panicIfGETCalledMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "PDF preview unavailable for this file type.") {
			t.Errorf("expected PDF-unavailable message, got body: %s", rec.Body.String())
		}
	})

	t.Run("presign error propagates raw (500)", func(t *testing.T) {
		docRepo := emptyDocRepo()
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{PageCount: 0, Filetype: "application/pdf", FileURL: "documents/x.pdf"}, nil
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{
			err: data.ErrInternal,
		})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("document dropdown list error propagates raw (500)", func(t *testing.T) {
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return nil, data.ErrInternal
		}
		e, jwtAuth := newChunkTestApp(docRepo, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/chunks", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
