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

// newKnowledgeTestApp builds an *echo.Echo wired with a DocumentRepo and
// AtomicKnowledgeRepo, for knowledge_handler tests.
func newKnowledgeTestApp(
	documentRepo *mockrepo.DocumentRepoerMock,
	knowledgeRepo *mockrepo.AtomicKnowledgeRepoerMock,
) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := &webhandler.WebRepos{
		DocumentRepo:  documentRepo,
		KnowledgeRepo: knowledgeRepo,
	}
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func TestKnowledgeListPage(t *testing.T) {
	emptyDocRepo := func() *mockrepo.DocumentRepoerMock {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return &dto.PaginationResults[model.Document]{}, nil
		}
		return repo
	}

	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newKnowledgeTestApp(emptyDocRepo(), &mockrepo.AtomicKnowledgeRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/knowledge", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("document dropdown list error propagates raw (500)", func(t *testing.T) {
		docRepo := &mockrepo.DocumentRepoerMock{}
		docRepo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return nil, data.ErrInternal
		}
		e, jwtAuth := newKnowledgeTestApp(docRepo, &mockrepo.AtomicKnowledgeRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("no document_id renders empty state", func(t *testing.T) {
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), &mockrepo.AtomicKnowledgeRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Select a document to view its knowledge.") {
			t.Errorf("expected empty-state message, got body: %s", rec.Body.String())
		}
	})

	t.Run("invalid document_id renders same empty state without calling svc.List", func(t *testing.T) {
		knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
		// PaginateByDocumentIDFunc intentionally left nil: if it's called,
		// the mock panics, proving the invalid-UUID branch short-circuits.
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), knowledgeRepo)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge?document_id=not-a-uuid", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Select a document to view its knowledge.") {
			t.Errorf("expected empty-state message, got body: %s", rec.Body.String())
		}
	})

	t.Run("valid document_id with no facts renders no-knowledge state", func(t *testing.T) {
		knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
		knowledgeRepo.PaginateByDocumentIDFunc = func(
			context.Context, uuid.UUID, data.SupportsPagination,
		) (*dto.PaginationResults[model.AtomicKnowledge], error) {
			return &dto.PaginationResults[model.AtomicKnowledge]{}, nil
		}
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), knowledgeRepo)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "No knowledge extracted for this document.") {
			t.Errorf("expected no-knowledge message, got body: %s", rec.Body.String())
		}
	})

	t.Run("facts present render table", func(t *testing.T) {
		knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
		knowledgeRepo.PaginateByDocumentIDFunc = func(
			context.Context, uuid.UUID, data.SupportsPagination,
		) (*dto.PaginationResults[model.AtomicKnowledge], error) {
			return &dto.PaginationResults[model.AtomicKnowledge]{
				TotalItems: 1,
				Items: []model.AtomicKnowledge{
					{ID: uuid.New(), Subject: "aspirin", Predicate: "reduces risk of", Object: "heart attack"},
				},
			}, nil
		}
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), knowledgeRepo)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "aspirin") {
			t.Errorf("expected fact subject in body, got: %s", rec.Body.String())
		}
	})

	t.Run("svc.List error propagates raw (500)", func(t *testing.T) {
		knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
		knowledgeRepo.PaginateByDocumentIDFunc = func(
			context.Context, uuid.UUID, data.SupportsPagination,
		) (*dto.PaginationResults[model.AtomicKnowledge], error) {
			return nil, data.ErrInternal
		}
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), knowledgeRepo)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing page query defaults to page 1 limit 20", func(t *testing.T) {
		var captured data.SupportsPagination
		knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
		knowledgeRepo.PaginateByDocumentIDFunc = func(
			_ context.Context, _ uuid.UUID, q data.SupportsPagination,
		) (*dto.PaginationResults[model.AtomicKnowledge], error) {
			captured = q
			return &dto.PaginationResults[model.AtomicKnowledge]{}, nil
		}
		e, jwtAuth := newKnowledgeTestApp(emptyDocRepo(), knowledgeRepo)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/knowledge?document_id="+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured == nil || captured.Offset() != 0 || captured.Limit() != 20 {
			t.Errorf("expected page=1/limit=20 defaults (offset=0), got %+v", captured)
		}
	})
}
