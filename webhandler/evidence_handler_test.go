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
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newEvidenceTestApp builds an *echo.Echo wired with a DocumentRepo,
// DocumentChunkRepo, and MediaProvider, for evidence_handler tests.
func newEvidenceTestApp(
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

func TestEvidenceViewPage(t *testing.T) {
	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newEvidenceTestApp(&mockrepo.DocumentRepoerMock{}, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/evidence", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("invalid document_id returns 400", func(t *testing.T) {
		e, jwtAuth := newEvidenceTestApp(&mockrepo.DocumentRepoerMock{}, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id=bad&chunk_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Invalid document_id.") {
			t.Errorf("expected invalid-document_id message, got body: %s", rec.Body.String())
		}
	})

	t.Run("invalid chunk_id returns 400", func(t *testing.T) {
		e, jwtAuth := newEvidenceTestApp(&mockrepo.DocumentRepoerMock{}, &mockrepo.DocumentChunkRepoerMock{}, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+uuid.New().String()+"&chunk_id=bad", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Invalid chunk_id.") {
			t.Errorf("expected invalid-chunk_id message, got body: %s", rec.Body.String())
		}
	})

	t.Run("chunk not found propagates raw (500, not 404)", func(t *testing.T) {
		// Unlike document_chunk_handler.go, evidence_handler.go does not
		// special-case data.ErrNotFound for the chunk lookup — it propagates
		// raw, which the custom error handler's default: branch collapses
		// to 500, not 404.
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return nil, data.ErrNotFound
		}
		e, jwtAuth := newEvidenceTestApp(&mockrepo.DocumentRepoerMock{}, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+uuid.New().String()+"&chunk_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("chunk belongs to a different document returns 404", func(t *testing.T) {
		docID := uuid.New()
		otherDocID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{ID: chunkID, DocumentID: otherDocID}, nil
		}
		e, jwtAuth := newEvidenceTestApp(&mockrepo.DocumentRepoerMock{}, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Chunk does not belong to that document.") {
			t.Errorf("expected ownership-mismatch message, got body: %s", rec.Body.String())
		}
	})

	t.Run("document lookup error propagates raw (500)", func(t *testing.T) {
		docID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{ID: chunkID, DocumentID: docID}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		e, jwtAuth := newEvidenceTestApp(docRepo, chunkRepo, &stubMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("non-PDF document returns 422", func(t *testing.T) {
		docID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{ID: chunkID, DocumentID: docID}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: docID, Filetype: "text/markdown"}, nil
		}
		e, jwtAuth := newEvidenceTestApp(docRepo, chunkRepo, &panicIfGETCalledMediaProvider{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Evidence viewing is only available for PDF documents.") {
			t.Errorf("expected non-PDF message, got body: %s", rec.Body.String())
		}
	})

	t.Run("presign error propagates raw (500)", func(t *testing.T) {
		docID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{ID: chunkID, DocumentID: docID}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: docID, Filetype: "application/pdf", FileURL: "documents/x.pdf"}, nil
		}
		e, jwtAuth := newEvidenceTestApp(docRepo, chunkRepo, &stubMediaProvider{err: data.ErrInternal})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success renders viewer with title and presigned URL", func(t *testing.T) {
		docID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{
				ID: chunkID, DocumentID: docID,
				ParsedChunk: model.ParsedChunk{Page: 3, BoundingBox: model.BoundingBox{0.1, 0.2, 0.3, 0.4}},
			}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{
				ID: docID, Filetype: "application/pdf", FileURL: "documents/x.pdf", Title: "My Evidence Doc",
			}, nil
		}
		e, jwtAuth := newEvidenceTestApp(docRepo, chunkRepo, &stubMediaProvider{presignedURL: "https://example.com/evidence-pdf"})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "My Evidence Doc") {
			t.Errorf("expected document title in body, got: %s", body)
		}
		if !strings.Contains(body, "https://example.com/evidence-pdf") {
			t.Errorf("expected presigned URL in body, got: %s", body)
		}
	})

	t.Run("falls back to filename when title is empty", func(t *testing.T) {
		docID := uuid.New()
		chunkID := uuid.New()
		chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
		chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
			return &model.DocumentChunk{ID: chunkID, DocumentID: docID}, nil
		}
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{
				ID: docID, Filetype: "application/pdf", FileURL: "documents/x.pdf",
				Title: "", Filename: "fallback-name.pdf",
			}, nil
		}
		e, jwtAuth := newEvidenceTestApp(docRepo, chunkRepo, &stubMediaProvider{presignedURL: "https://example.com/x"})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/evidence?document_id="+docID.String()+"&chunk_id="+chunkID.String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "fallback-name.pdf") {
			t.Errorf("expected filename fallback in body, got: %s", rec.Body.String())
		}
	})
}
