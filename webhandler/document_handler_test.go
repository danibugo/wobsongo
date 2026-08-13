package webhandler_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/labstack/echo/v4"
)

// newDocTestApp builds an *echo.Echo wired with only a DocumentRepo and raw
// object store, for document_handler tests — UserRepo is irrelevant here
// except that every route under test requires authentication.
func newDocTestApp(documentRepo *mockrepo.DocumentRepoerMock, rawStore data.RawObjectStore) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := newWebRepos(nil, documentRepo, rawStore)
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func TestDocumentRoutes_Unauthenticated_RedirectToLogin(t *testing.T) {
	e, _ := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

	id := uuid.New().String()
	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/documents"},
		{http.MethodGet, "/documents/new"},
		{http.MethodPost, "/documents/new"},
		{http.MethodGet, "/documents/" + id + "/edit"},
		{http.MethodPost, "/documents/" + id + "/edit"},
		{http.MethodPost, "/documents/" + id + "/delete"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(r.method, r.target, nil))

			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != "/login" {
				t.Errorf("Location = %q, want %q", got, "/login")
			}
		})
	}
}

func TestListPage(t *testing.T) {
	t.Run("success renders results", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return &dto.PaginationResults[model.Document]{
				Page: 1, PerPage: 20, TotalItems: 1,
				Items: []model.Document{{ID: uuid.New(), Title: "A Fake Document"}},
			}, nil
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "A Fake Document") {
			t.Errorf("expected document title in body, got: %s", rec.Body.String())
		}
	})

	t.Run("defaults page and per_page on missing query", func(t *testing.T) {
		var captured data.SupportsPagination
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(_ context.Context, q data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			captured = q
			return &dto.PaginationResults[model.Document]{}, nil
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		// SupportsPagination only exposes Limit()/Offset() — page 1 with
		// limit 20 means Offset() == 0.
		if captured == nil || captured.Offset() != 0 || captured.Limit() != 20 {
			t.Errorf("expected page=1/limit=20 defaults (offset=0), got %+v", captured)
		}
	})

	t.Run("defaults on invalid page query", func(t *testing.T) {
		var captured data.SupportsPagination
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(_ context.Context, q data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			captured = q
			return &dto.PaginationResults[model.Document]{}, nil
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents?page=not-a-number", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured == nil || captured.Offset() != 0 {
			t.Errorf("expected page to default to 1 (offset=0), got %+v", captured)
		}
	})

	t.Run("propagated repo error returns 500", func(t *testing.T) {
		// webhandler has no equivalent of handler's JSON-API error-to-status
		// mapping — listPage returns the raw error, and Echo's/webhandler's
		// custom error handler only special-cases *model.APIError and
		// *echo.HTTPError, so any data.Err* sentinel collapses to 500.
		repo := &mockrepo.DocumentRepoerMock{}
		repo.PaginateFunc = func(context.Context, data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return nil, data.ErrNotFound
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestNewPage(t *testing.T) {
	e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

	req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents/new", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Upload document") {
		t.Error("expected the create-document form to be rendered")
	}
}

func TestCreatePost(t *testing.T) {
	validFields := func() map[string]string {
		return map[string]string{"title": "My Doc", "publisher": "Acme", "language": "en", "year": "2020"}
	}

	t.Run("missing file rerenders with error", func(t *testing.T) {
		e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

		req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "", nil)
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Please select a file to upload.") {
			t.Errorf("expected missing-file error, got body: %s", rec.Body.String())
		}
	})

	t.Run("disallowed extension rerenders with error", func(t *testing.T) {
		e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

		req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "malware.exe", []byte("x"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		// templ HTML-escapes the message text, so the literal quotes around
		// the extension render as &#34; entities.
		if !strings.Contains(rec.Body.String(), `File type &#34;.exe&#34; is not supported.`) {
			t.Errorf("expected disallowed-extension error, got body: %s", rec.Body.String())
		}
	})

	t.Run("upload failure rerenders with error and never reaches Create", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		// GetBySHA256Func/CreateFunc intentionally left nil: if either is
		// called, the mock panics, proving the upload failure short-circuits
		// before reaching them.
		rawStore := &stubRawObjectStore{
			putObjectFunc: func(context.Context, string, io.Reader, int64, string) error {
				return context.DeadlineExceeded
			},
		}
		e, jwtAuth := newDocTestApp(repo, rawStore)

		req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "doc.pdf", []byte("%PDF-1.4"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Failed to upload file. Please try again.") {
			t.Errorf("expected upload-failure error, got body: %s", rec.Body.String())
		}
	})

	t.Run("invalid language rerenders with error", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		fields := validFields()
		fields["language"] = "de"
		req := newMultipartUploadRequest(t, "/documents/new", fields, "file", "doc.pdf", []byte("%PDF-1.4"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Please select a language.") {
			t.Errorf("expected invalid-language error, got body: %s", rec.Body.String())
		}
	})

	t.Run("service error rerenders with message regardless of error type", func(t *testing.T) {
		tests := []struct {
			name    string
			mutate  func(repo *mockrepo.DocumentRepoerMock)
			wantErr string
		}{
			{
				name: "WithTx fails",
				mutate: func(repo *mockrepo.DocumentRepoerMock) {
					repo.WithTxFunc = func(context.Context, func(data.DocumentRepoer) error) error {
						return data.ErrInternal
					}
				},
				wantErr: data.ErrInternal.Error(),
			},
			{
				name: "Enqueue fails after Create succeeds",
				mutate: func(repo *mockrepo.DocumentRepoerMock) {
					repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
						return fn(repo)
					}
					repo.CreateFunc = func(context.Context, *model.Document) error { return nil }
					repo.EnqueueFunc = func(context.Context, queue.BackgroundJob) error { return data.ErrInternal }
				},
				wantErr: data.ErrInternal.Error(),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := &mockrepo.DocumentRepoerMock{}
				repo.GetBySHA256Func = func(context.Context, string) (*model.Document, error) {
					return nil, data.ErrNotFound
				}
				tt.mutate(repo)
				e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

				req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "doc.pdf", []byte("%PDF-1.4"))
				req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "Failed to save document: "+tt.wantErr) {
					t.Errorf("expected save-failure error, got body: %s", rec.Body.String())
				}
			})
		}
	})

	t.Run("duplicate SHA256 is an idempotent success", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetBySHA256Func = func(context.Context, string) (*model.Document, error) {
			return &model.Document{ID: uuid.New()}, nil
		}
		// WithTxFunc/CreateFunc/EnqueueFunc intentionally left nil — calling
		// any of them would panic, proving the idempotent short-circuit.
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "doc.pdf", []byte("%PDF-1.4"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/documents" {
			t.Errorf("Location = %q, want %q", got, "/documents")
		}
	})

	t.Run("success redirects to documents", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetBySHA256Func = func(context.Context, string) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
			return fn(repo)
		}
		repo.CreateFunc = func(context.Context, *model.Document) error { return nil }
		repo.EnqueueFunc = func(context.Context, queue.BackgroundJob) error { return nil }

		var putKey string
		rawStore := &stubRawObjectStore{
			putObjectFunc: func(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
				putKey = key
				return nil
			},
		}
		e, jwtAuth := newDocTestApp(repo, rawStore)

		req := newMultipartUploadRequest(t, "/documents/new", validFields(), "file", "doc.pdf", []byte("%PDF-1.4"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/documents" {
			t.Errorf("Location = %q, want %q", got, "/documents")
		}
		if !strings.HasPrefix(putKey, "documents/") || !strings.HasSuffix(putKey, ".pdf") {
			t.Errorf("expected s3 key documents/<sha256>.pdf, got %q", putKey)
		}
	})

	t.Run("success with no year provided defaults to zero", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetBySHA256Func = func(context.Context, string) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
			return fn(repo)
		}
		var created *model.Document
		repo.CreateFunc = func(_ context.Context, doc *model.Document) error {
			created = doc
			return nil
		}
		repo.EnqueueFunc = func(context.Context, queue.BackgroundJob) error { return nil }
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		fields := validFields()
		fields["year"] = ""
		req := newMultipartUploadRequest(t, "/documents/new", fields, "file", "doc.pdf", []byte("%PDF-1.4"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if created == nil || created.PublicationYear != 0 {
			t.Errorf("expected PublicationYear 0 when omitted, got %+v", created)
		}
	})
}

func TestEditPage(t *testing.T) {
	t.Run("invalid uuid returns 404", func(t *testing.T) {
		e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents/not-a-uuid/edit", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Document not found.") {
			t.Errorf("expected not-found message, got body: %s", rec.Body.String())
		}
	})

	t.Run("success renders form with document", func(t *testing.T) {
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK)
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: id, Title: "Existing Doc"}, nil
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents/"+id.String()+"/edit", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Existing Doc") {
			t.Errorf("expected document title in body, got: %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Save changes") {
			t.Error("expected the edit-document form to be rendered")
		}
	})

	t.Run("propagated repo error returns 500 regardless of sentinel", func(t *testing.T) {
		suffixes := []string{
			testhelpers.SuffixNotFound,
			testhelpers.SuffixForbidden,
			testhelpers.SuffixInternal,
			testhelpers.SuffixConflict,
		}
		for _, suffix := range suffixes {
			t.Run(suffix, func(t *testing.T) {
				id := testhelpers.NewUUIDWithSuffix(suffix)
				repo := &mockrepo.DocumentRepoerMock{}
				repo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.Document, error) {
					return nil, testhelpers.ErrorForUUID(id)
				}
				e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

				req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/documents/"+id.String()+"/edit", nil)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusInternalServerError {
					t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})
}

func TestUpdatePost(t *testing.T) {
	t.Run("invalid uuid returns 404", func(t *testing.T) {
		e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodPost, "/documents/not-a-uuid/edit", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing title rerenders with error", func(t *testing.T) {
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK)
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: id, Title: "Existing Doc"}, nil
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := formRequest("/documents/"+id.String()+"/edit", urlValuesFor("", "Acme", "2020"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Title is required.") {
			t.Errorf("expected title-required error, got body: %s", rec.Body.String())
		}
	})

	t.Run("missing title and re-fetch also fails propagates raw error (500, not 200)", func(t *testing.T) {
		// The distinguishing edge case: renderErr's internal re-fetch (for
		// form pre-fill) can itself fail, breaking the otherwise-universal
		// "always 200" swallow pattern for this handler.
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixNotFound)
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.Document, error) {
			return nil, testhelpers.ErrorForUUID(id)
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := formRequest("/documents/"+id.String()+"/edit", urlValuesFor("", "Acme", "2020"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 (re-fetch failure propagates raw), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("service error rerenders with message regardless of error type", func(t *testing.T) {
		for _, svcErr := range []error{data.ErrInternal, data.ErrConflict} {
			t.Run(svcErr.Error(), func(t *testing.T) {
				id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK)
				repo := &mockrepo.DocumentRepoerMock{}
				repo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
					return &model.Document{ID: id, Title: "Existing Doc"}, nil
				}
				repo.UpdateFunc = func(context.Context, *model.Document) error { return svcErr }
				e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

				req := formRequest("/documents/"+id.String()+"/edit", urlValuesFor("New Title", "Acme", "2020"))
				req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "Failed to update document: "+svcErr.Error()) {
					t.Errorf("expected update-failure error, got body: %s", rec.Body.String())
				}
			})
		}
	})

	t.Run("success redirects to documents", func(t *testing.T) {
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK)
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.Document, error) {
			return &model.Document{ID: id, Title: "Existing Doc"}, nil
		}
		repo.UpdateFunc = func(context.Context, *model.Document) error { return nil }
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := formRequest("/documents/"+id.String()+"/edit", urlValuesFor("New Title", "Acme", "2020"))
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{ID: uuid.New(), Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/documents" {
			t.Errorf("Location = %q, want %q", got, "/documents")
		}
	})
}

func TestDeletePost(t *testing.T) {
	t.Run("invalid uuid returns 404", func(t *testing.T) {
		e, jwtAuth := newDocTestApp(&mockrepo.DocumentRepoerMock{}, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodPost, "/documents/not-a-uuid/delete", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success redirects to documents", func(t *testing.T) {
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK)
		repo := &mockrepo.DocumentRepoerMock{}
		repo.DeleteFunc = func(_ context.Context, id uuid.UUID) error {
			return testhelpers.ErrorForUUID(id)
		}
		e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

		req := newAuthedRequest(t, jwtAuth, http.MethodPost, "/documents/"+id.String()+"/delete", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/documents" {
			t.Errorf("Location = %q, want %q", got, "/documents")
		}
	})

	t.Run("propagated repo error returns 500 regardless of sentinel, no swallowing", func(t *testing.T) {
		suffixes := []string{
			testhelpers.SuffixNotFound,
			testhelpers.SuffixForbidden,
			testhelpers.SuffixInternal,
			testhelpers.SuffixConflict,
		}
		for _, suffix := range suffixes {
			t.Run(suffix, func(t *testing.T) {
				id := testhelpers.NewUUIDWithSuffix(suffix)
				repo := &mockrepo.DocumentRepoerMock{}
				repo.DeleteFunc = func(_ context.Context, id uuid.UUID) error {
					return testhelpers.ErrorForUUID(id)
				}
				e, jwtAuth := newDocTestApp(repo, &stubRawObjectStore{})

				req := newAuthedRequest(t, jwtAuth, http.MethodPost, "/documents/"+id.String()+"/delete", nil)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusInternalServerError {
					t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})
}

func urlValuesFor(title, publisher, year string) url.Values {
	return url.Values{"title": {title}, "publisher": {publisher}, "year": {year}}
}
