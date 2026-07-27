package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
)

func TestDocumentService_Create(t *testing.T) {
	var created *model.Document
	var enqueued queue.BackgroundJob
	repo := &mockrepo.DocumentRepoerMock{}
	repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
		return fn(repo)
	}
	repo.GetBySHA256Func = func(_ context.Context, _ string) (*model.Document, error) {
		return nil, data.ErrNotFound
	}
	repo.CreateFunc = func(_ context.Context, entity *model.Document) error {
		created = entity
		return nil
	}
	repo.EnqueueFunc = func(_ context.Context, payload queue.BackgroundJob) error {
		enqueued = payload
		return nil
	}
	svc := service.NewDocumentService(repo)

	req := &dto.CreateDocumentDTO{
		SHA256:          "abc123",
		FileKey:         "documents/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf",
		Title:           "A Fake Document",
		Filename:        "fake.pdf",
		Filetype:        "application/pdf",
		Filesize:        1024,
		PageCount:       10,
		PublisherName:   "Fake Press",
		PublicationYear: 2020,
		Language:        "en",
	}

	doc, err := svc.Create(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID == uuid.Nil {
		t.Error("expected a generated document ID")
	}
	if doc.Title != req.Title || doc.SHA256 != req.SHA256 || string(doc.FileURL) != req.FileKey {
		t.Errorf("document fields do not match request: %+v", doc)
	}
	if created != doc {
		t.Error("expected the same document passed to repo.Create to be returned")
	}

	parseJob, ok := enqueued.(queue.ParseDocumentDTO)
	if !ok {
		t.Fatalf("expected a queue.ParseDocumentDTO to be enqueued, got %T", enqueued)
	}
	if parseJob.DocumentID != doc.ID {
		t.Errorf("expected enqueued job DocumentID %s, got %s", doc.ID, parseJob.DocumentID)
	}
	if parseJob.FileKey != req.FileKey {
		t.Errorf("expected enqueued job FileKey %s, got %s", req.FileKey, parseJob.FileKey)
	}
}

func TestDocumentService_Create_PropagatesRepoError(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{}
	repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
		return fn(repo)
	}
	repo.GetBySHA256Func = func(_ context.Context, _ string) (*model.Document, error) {
		return nil, data.ErrNotFound
	}
	repo.CreateFunc = func(_ context.Context, _ *model.Document) error { return data.ErrInternal }
	svc := service.NewDocumentService(repo)

	_, err := svc.Create(t.Context(), &dto.CreateDocumentDTO{
		FileKey:  "documents/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf",
		Language: "en",
	})
	if !errors.Is(err, data.ErrInternal) {
		t.Errorf("expected data.ErrInternal, got %v", err)
	}
}

func TestDocumentService_Create_DuplicateSHA256_ReturnsExistingNoOp(t *testing.T) {
	existing := &model.Document{ID: uuid.New(), SHA256: "abc123", Title: "Already Ingested"}
	repo := &mockrepo.DocumentRepoerMock{
		GetBySHA256Func: func(_ context.Context, sha256 string) (*model.Document, error) {
			if sha256 != existing.SHA256 {
				t.Errorf("expected GetBySHA256 called with %s, got %s", existing.SHA256, sha256)
			}
			return existing, nil
		},
		CreateFunc: func(_ context.Context, _ *model.Document) error {
			t.Error("expected Create not to be called for a duplicate SHA256")
			return nil
		},
		EnqueueFunc: func(_ context.Context, _ queue.BackgroundJob) error {
			t.Error("expected Enqueue not to be called for a duplicate SHA256")
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	doc, err := svc.Create(t.Context(), &dto.CreateDocumentDTO{SHA256: existing.SHA256})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc != existing {
		t.Errorf("expected the existing document to be returned, got %+v", doc)
	}
}

func TestDocumentService_Create_ConcurrentDuplicate_ReturnsExistingNoOp(t *testing.T) {
	existing := &model.Document{ID: uuid.New(), SHA256: "abc123", Title: "Won The Race"}
	getCalls := 0
	repo := &mockrepo.DocumentRepoerMock{}
	repo.WithTxFunc = func(ctx context.Context, fn func(data.DocumentRepoer) error) error {
		return fn(repo)
	}
	repo.GetBySHA256Func = func(_ context.Context, _ string) (*model.Document, error) {
		getCalls++
		if getCalls == 1 {
			return nil, data.ErrNotFound
		}
		return existing, nil
	}
	repo.CreateFunc = func(_ context.Context, _ *model.Document) error { return data.ErrConflict }
	svc := service.NewDocumentService(repo)

	doc, err := svc.Create(t.Context(), &dto.CreateDocumentDTO{
		SHA256:   existing.SHA256,
		FileKey:  "documents/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pdf",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc != existing {
		t.Errorf("expected the existing document to be returned, got %+v", doc)
	}
	if getCalls != 2 {
		t.Errorf(
			"expected GetBySHA256 to be called twice (pre-check + post-conflict), got %d",
			getCalls,
		)
	}
}

func TestDocumentService_GetByID(t *testing.T) {
	id := uuid.New()
	want := &model.Document{ID: id, Title: "Found"}
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, gotID uuid.UUID) (*model.Document, error) {
			if gotID != id {
				t.Errorf("expected repo.GetByID called with %s, got %s", id, gotID)
			}
			return want, nil
		},
	}
	svc := service.NewDocumentService(repo)

	got, err := svc.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestDocumentService_GetByID_PropagatesNotFound(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) {
			return nil, data.ErrNotFound
		},
	}
	svc := service.NewDocumentService(repo)

	_, err := svc.GetByID(t.Context(), uuid.New())
	if !errors.Is(err, data.ErrNotFound) {
		t.Errorf("expected data.ErrNotFound, got %v", err)
	}
}

func TestDocumentService_List(t *testing.T) {
	want := &dto.PaginationResults[model.Document]{
		Page: 1, PerPage: 20, TotalItems: 1,
		Items: []model.Document{{ID: uuid.New()}},
	}
	repo := &mockrepo.DocumentRepoerMock{
		PaginateFunc: func(_ context.Context, _ data.SupportsPagination) (*dto.PaginationResults[model.Document], error) {
			return want, nil
		},
	}
	svc := service.NewDocumentService(repo)

	got, err := svc.List(t.Context(), &dto.PaginationDTO{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestDocumentService_Update(t *testing.T) {
	id := uuid.New()
	existing := &model.Document{ID: id, Title: "Old Title", PublisherName: "Old Press"}
	var updated *model.Document
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) { return existing, nil },
		UpdateFunc: func(_ context.Context, entity *model.Document) error {
			updated = entity
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	req := &dto.UpdateDocumentDTO{
		Title:           "New Title",
		PublisherName:   "New Press",
		PublicationYear: 2021,
	}
	doc, err := svc.Update(t.Context(), id, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Title != req.Title || doc.PublisherName != req.PublisherName ||
		doc.PublicationYear != req.PublicationYear {
		t.Errorf("expected fields to be updated, got %+v", doc)
	}
	if updated != existing {
		t.Error("expected repo.Update to be called with the mutated existing document")
	}
}

func TestDocumentService_Update_NotFoundNeverCallsUpdate(t *testing.T) {
	updateCalled := false
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) { return nil, data.ErrNotFound },
		UpdateFunc: func(_ context.Context, _ *model.Document) error {
			updateCalled = true
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	_, err := svc.Update(
		t.Context(),
		uuid.New(),
		&dto.UpdateDocumentDTO{Title: "New Title"},
	)
	if !errors.Is(err, data.ErrNotFound) {
		t.Errorf("expected data.ErrNotFound, got %v", err)
	}
	if updateCalled {
		t.Error("expected repo.Update not to be called when GetByID fails")
	}
}

func TestDocumentService_Delete(t *testing.T) {
	id := uuid.New()
	repo := &mockrepo.DocumentRepoerMock{
		DeleteFunc: func(_ context.Context, gotID uuid.UUID) error {
			if gotID != id {
				t.Errorf("expected repo.Delete called with %s, got %s", id, gotID)
			}
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	if err := svc.Delete(t.Context(), id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocumentService_Delete_PropagatesRepoError(t *testing.T) {
	repo := &mockrepo.DocumentRepoerMock{
		DeleteFunc: func(_ context.Context, _ uuid.UUID) error { return data.ErrForbidden },
	}
	svc := service.NewDocumentService(repo)

	err := svc.Delete(t.Context(), uuid.New())
	if !errors.Is(err, data.ErrForbidden) {
		t.Errorf("expected data.ErrForbidden, got %v", err)
	}
}

func TestDocumentService_UpdateAfterParse(t *testing.T) {
	id := uuid.New()
	existing := &model.Document{ID: id, Title: "Existing", PageCount: 0}
	var updated *model.Document
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) { return existing, nil },
		UpdateFunc: func(_ context.Context, entity *model.Document) error {
			updated = entity
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	if err := svc.UpdateAfterParse(t.Context(), id, 42, "Docling's Title"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.PageCount != 42 {
		t.Errorf("expected page count 42, got %d", updated.PageCount)
	}
	if updated.Title != "Existing" {
		t.Errorf("expected existing title to be preserved, got %q", updated.Title)
	}
	if updated != existing {
		t.Error("expected repo.Update to be called with the mutated existing document")
	}
}

func TestDocumentService_UpdateAfterParse_BackfillsBlankTitle(t *testing.T) {
	id := uuid.New()
	existing := &model.Document{ID: id, Title: "", PageCount: 0}
	var updated *model.Document
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) { return existing, nil },
		UpdateFunc: func(_ context.Context, entity *model.Document) error {
			updated = entity
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	if err := svc.UpdateAfterParse(t.Context(), id, 7, "Docling's Title"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Title != "Docling's Title" {
		t.Errorf("expected blank title to be backfilled from Docling, got %q", updated.Title)
	}
}

func TestDocumentService_UpdateAfterParse_NotFoundNeverCallsUpdate(t *testing.T) {
	updateCalled := false
	repo := &mockrepo.DocumentRepoerMock{
		GetByIDFunc: func(_ context.Context, _ uuid.UUID) (*model.Document, error) { return nil, data.ErrNotFound },
		UpdateFunc: func(_ context.Context, _ *model.Document) error {
			updateCalled = true
			return nil
		},
	}
	svc := service.NewDocumentService(repo)

	err := svc.UpdateAfterParse(t.Context(), uuid.New(), 10, "Some Title")
	if !errors.Is(err, data.ErrNotFound) {
		t.Errorf("expected data.ErrNotFound, got %v", err)
	}
	if updateCalled {
		t.Error("expected repo.Update not to be called when GetByID fails")
	}
}
