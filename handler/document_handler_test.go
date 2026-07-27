package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/core"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/labstack/echo/v4"
)

const documentsBasePath = "/api/v1/documents"

func newDocumentTestApp(repo data.DocumentRepoer) *echo.Echo {
	app := core.NewApp(testhelpers.NewEcho(), config.NewConfig(), core.WithDocumentRepo(repo))
	return app.Echo()
}

// validSHA256 is the SHA-256 digest of the empty string, used as a
// well-formed 64-hex-char fixture for the s3key-validated FileKey field.
const validSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func validCreateDocumentBody() dto.CreateDocumentDTO {
	return dto.CreateDocumentDTO{
		SHA256:          validSHA256,
		FileKey:         "documents/" + validSHA256 + ".pdf",
		Title:           "A Fake Document",
		Filename:        "fake.pdf",
		Filetype:        "application/pdf",
		Filesize:        1024,
		PageCount:       10,
		PublisherName:   "Fake Press",
		PublicationYear: 2020,
		Language:        "en",
	}
}

func TestCreateDocumentHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetBySHA256Func = func(_ context.Context, _ string) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
			return fn(repo)
		}
		repo.CreateFunc = func(_ context.Context, _ *model.Document) error { return nil }
		repo.EnqueueFunc = func(_ context.Context, _ queue.BackgroundJob) error { return nil }
		app := newDocumentTestApp(repo)

		body, err := json.Marshal(validCreateDocumentBody())
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, documentsBasePath, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf(
				"expected status %d, got %d: %s",
				http.StatusCreated,
				rec.Code,
				rec.Body.String(),
			)
		}

		var resp testhelpers.APIResponse[model.Document]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response body: %v", err)
		}
		if resp.Data.ID == uuid.Nil {
			t.Error("expected a generated document ID")
		}
		if resp.Data.Title != "A Fake Document" {
			t.Errorf("expected title %q, got %q", "A Fake Document", resp.Data.Title)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		app := newDocumentTestApp(&mockrepo.DocumentRepoerMock{})

		req := httptest.NewRequest(
			http.MethodPost,
			documentsBasePath,
			bytes.NewReader([]byte("{not valid json")),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf(
				"expected status %d, got %d: %s",
				http.StatusBadRequest,
				rec.Code,
				rec.Body.String(),
			)
		}
	})

	t.Run("ValidationFailure_MissingTitle", func(t *testing.T) {
		app := newDocumentTestApp(&mockrepo.DocumentRepoerMock{})

		form := validCreateDocumentBody()
		form.Title = ""
		body, err := json.Marshal(form)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, documentsBasePath, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf(
				"expected status %d, got %d: %s",
				http.StatusUnprocessableEntity,
				rec.Code,
				rec.Body.String(),
			)
		}
	})

	t.Run("ValidationFailure_InvalidFileKey", func(t *testing.T) {
		app := newDocumentTestApp(&mockrepo.DocumentRepoerMock{})

		form := validCreateDocumentBody()
		form.FileKey = "docs/not-a-valid-key.pdf"
		body, err := json.Marshal(form)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, documentsBasePath, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf(
				"expected status %d, got %d: %s",
				http.StatusUnprocessableEntity,
				rec.Code,
				rec.Body.String(),
			)
		}
	})

	t.Run("RepoInternalError", func(t *testing.T) {
		repo := &mockrepo.DocumentRepoerMock{}
		repo.GetBySHA256Func = func(_ context.Context, _ string) (*model.Document, error) {
			return nil, data.ErrNotFound
		}
		repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
			return fn(repo)
		}
		repo.CreateFunc = func(_ context.Context, _ *model.Document) error { return data.ErrInternal }
		app := newDocumentTestApp(repo)

		body, err := json.Marshal(validCreateDocumentBody())
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, documentsBasePath, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf(
				"expected status %d, got %d: %s",
				http.StatusInternalServerError,
				rec.Code,
				rec.Body.String(),
			)
		}
	})
}

func TestGetDocumentHandler(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, id uuid.UUID) (*model.Document, error) {
			if err := testhelpers.ErrorForUUID(id); err != nil {
				return nil, err
			}
			return &model.Document{ID: id, Title: "Found Document"}, nil
		},
	}
	app := newDocumentTestApp(repo)

	cases := []struct {
		name           string
		id             string
		expectedStatus int
	}{
		{"Success", testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK).String(), http.StatusOK},
		{
			"NotFound",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixNotFound).String(),
			http.StatusNotFound,
		},
		{
			"Forbidden",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixForbidden).String(),
			http.StatusForbidden,
		},
		{
			"InternalError",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixInternal).String(),
			http.StatusInternalServerError,
		},
		{"InvalidUUID", "not-a-uuid", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, documentsBasePath+"/"+tc.id, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf(
					"expected status %d, got %d: %s",
					tc.expectedStatus,
					rec.Code,
					rec.Body.String(),
				)
			}
			if tc.expectedStatus == http.StatusOK {
				var resp testhelpers.APIResponse[model.Document]
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response body: %v", err)
				}
				if resp.Data.ID.String() != tc.id {
					t.Errorf("expected document ID %s, got %s", tc.id, resp.Data.ID)
				}
			}
		})
	}
}

func TestListDocumentsHandler(t *testing.T) {
	var capturedQuery data.SupportsPagination
	repo := &mockrepo.DocumentRepoerMock{
		PaginateFunc: func(_ context.Context, q data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			capturedQuery = q
			return &dto.PaginationResults[model.Document]{
				Page:       2,
				PerPage:    5,
				TotalItems: 1,
				Items:      []model.Document{{ID: uuid.New(), Title: "Doc"}},
			}, nil
		},
	}
	app := newDocumentTestApp(repo)

	req := httptest.NewRequest(http.MethodGet, documentsBasePath+"?page=2&per_page=5", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if capturedQuery == nil || capturedQuery.Offset() != 5 {
		t.Errorf("expected offset 5 to reach the repo, got %v", capturedQuery)
	}

	var resp testhelpers.APIResponse[dto.PaginationResults[model.Document]]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if len(resp.Data.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data.Items))
	}
}

func TestUpdateDocumentHandler(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, id uuid.UUID) (*model.Document, error) {
			if err := testhelpers.ErrorForUUID(id); err != nil {
				return nil, err
			}
			return &model.Document{ID: id, Title: "Old Title"}, nil
		},
		UpdateFunc: func(_ context.Context, entity *model.Document) error {
			return testhelpers.ErrorForUUID(entity.ID)
		},
	}
	app := newDocumentTestApp(repo)

	cases := []struct {
		name           string
		id             string
		expectedStatus int
	}{
		{"Success", testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK).String(), http.StatusOK},
		{
			"NotFound",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixNotFound).String(),
			http.StatusNotFound,
		},
		{
			"Forbidden",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixForbidden).String(),
			http.StatusForbidden,
		},
		{
			"Conflict",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixConflict).String(),
			http.StatusConflict,
		},
		{
			"InternalError",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixInternal).String(),
			http.StatusInternalServerError,
		},
		{"InvalidUUID", "not-a-uuid", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(dto.UpdateDocumentDTO{Title: "New Title"})
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}

			req := httptest.NewRequest(
				http.MethodPut,
				documentsBasePath+"/"+tc.id,
				bytes.NewReader(body),
			)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf(
					"expected status %d, got %d: %s",
					tc.expectedStatus,
					rec.Code,
					rec.Body.String(),
				)
			}
		})
	}

	t.Run("ValidationFailure_MissingTitle", func(t *testing.T) {
		id := testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK).String()
		body, err := json.Marshal(dto.UpdateDocumentDTO{})
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}

		req := httptest.NewRequest(http.MethodPut, documentsBasePath+"/"+id, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf(
				"expected status %d, got %d: %s",
				http.StatusUnprocessableEntity,
				rec.Code,
				rec.Body.String(),
			)
		}
	})
}

func TestDeleteDocumentHandler(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{
		DeleteFunc: func(_ context.Context, id uuid.UUID) error {
			return testhelpers.ErrorForUUID(id)
		},
	}
	app := newDocumentTestApp(repo)

	cases := []struct {
		name           string
		id             string
		expectedStatus int
	}{
		{
			"Success",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixOK).String(),
			http.StatusNoContent,
		},
		{
			"NotFound",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixNotFound).String(),
			http.StatusNotFound,
		},
		{
			"Forbidden",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixForbidden).String(),
			http.StatusForbidden,
		},
		{
			"InternalError",
			testhelpers.NewUUIDWithSuffix(testhelpers.SuffixInternal).String(),
			http.StatusInternalServerError,
		},
		{"InvalidUUID", "not-a-uuid", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, documentsBasePath+"/"+tc.id, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf(
					"expected status %d, got %d: %s",
					tc.expectedStatus,
					rec.Code,
					rec.Body.String(),
				)
			}
		})
	}
}
