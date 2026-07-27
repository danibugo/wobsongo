// Package service defines business logic needed to handle
// data operations across the system. It clearly separates the
// transport layer and data leyer, by relying on the data package
// that defines the data storage layer as interfaces.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/validation"
)

// DocumentService defines a set of available methods
// related to documents operations.
type DocumentService struct {
	repo data.DocumentRepoer
}

// NewDocumentService creates a new DocumentService.
func NewDocumentService(repo data.DocumentRepoer) *DocumentService {
	return &DocumentService{
		repo: repo,
	}
}

// Create ingests a new document. If a document with the same SHA256 already
// exists, Create is idempotent: it returns the existing document rather than
// inserting a duplicate or re-enqueueing a parse job.
func (s *DocumentService) Create(
	ctx context.Context,
	req *dto.CreateDocumentDTO,
) (*model.Document, error) {
	if existing, err := s.repo.GetBySHA256(ctx, req.SHA256); err == nil {
		return existing, nil
	} else if !errors.Is(err, data.ErrNotFound) {
		return nil, err
	}

	// Re-checked here, not just via CreateDocumentDTO's `s3key` validator
	// tag, because that tag is only enforced by the HTTP handler's
	// c.Validate(req) — the CLI (cmd/document_insert.go) calls Create
	// directly and never runs the DTO through a validator at all. Without
	// this, a malformed FileKey (confirmed in production: a UUID-style key
	// left over from before this codebase settled on SHA256-based keys)
	// gets a document row and a ParseDocumentDTO enqueued that can never
	// succeed — Docling presigning rejects it every single retry, forever.
	// Failing fast here means a bad key never reaches the queue at all.
	if !validation.ValidateS3PrefixAndFile(req.FileKey) {
		return nil, fmt.Errorf("invalid or malformed file key: %s", req.FileKey)
	}

	// req.Language is already constrained to "en"/"fr" by CreateDocumentDTO's
	// validate:"oneof=en fr" tag, so this should never actually error — but
	// handled properly rather than ignored, since ParseLanguage is the
	// single source of truth for what's a valid language.
	language, err := model.ParseLanguage(req.Language)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	doc := &model.Document{
		ID:              uuid.New(),
		CreatedAt:       now,
		ModifiedAt:      now,
		FileURL:         model.S3Key(req.FileKey),
		SHA256:          req.SHA256,
		Title:           req.Title,
		Filename:        req.Filename,
		Filetype:        req.Filetype,
		Filesize:        req.Filesize,
		PageCount:       req.PageCount,
		PublisherName:   req.PublisherName,
		PublicationYear: req.PublicationYear,
		Language:        language,
	}

	err = s.repo.WithTx(ctx, func(txRepo data.DocumentRepoer) error {
		if err := txRepo.Create(ctx, doc); err != nil {
			return err
		}
		return txRepo.Enqueue(ctx, queue.ParseDocumentDTO{
			DocumentID: doc.ID,
			FileKey:    string(doc.FileURL),
		})
	})
	if err != nil {
		// A concurrent Create for the same SHA256 may have won the race
		// against the unique index between our pre-check and this insert;
		// treat that the same as the pre-check finding it, rather than
		// surfacing a conflict for what is, from the caller's perspective,
		// an idempotent no-op.
		if errors.Is(err, data.ErrConflict) {
			if existing, getErr := s.repo.GetBySHA256(ctx, req.SHA256); getErr == nil {
				return existing, nil
			}
		}
		return nil, err
	}

	return doc, nil
}

// GetBySHA256 retrieves a document by its content hash.
func (s *DocumentService) GetBySHA256(ctx context.Context, sha256 string) (*model.Document, error) {
	return s.repo.GetBySHA256(ctx, sha256)
}

// GetByID retrieves a document by its ID.
func (s *DocumentService) GetByID(ctx context.Context, id uuid.UUID) (*model.Document, error) {
	return s.repo.GetByID(ctx, id)
}

// List retrieves a paginated list of documents.
func (s *DocumentService) List(
	ctx context.Context,
	pagination *dto.PaginationDTO,
) (*dto.PaginationResults[model.Document], error) {
	return s.repo.Paginate(ctx, pagination)
}

// Update updates a document's descriptive metadata.
func (s *DocumentService) Update(
	ctx context.Context,
	id uuid.UUID,
	req *dto.UpdateDocumentDTO,
) (*model.Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	doc.Title = req.Title
	doc.PublisherName = req.PublisherName
	doc.PublicationYear = req.PublicationYear
	doc.ModifiedAt = time.Now()

	if err := s.repo.Update(ctx, doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// Delete removes a document by its ID.
func (s *DocumentService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// UpdateAfterParse backfills fields the ingestion pipeline only learns once
// Docling has actually parsed the document. PageCount is always overwritten
// (the document is created with page_count=0 up front; Docling is the sole
// source of truth for it). Title is only backfilled when the document
// doesn't already have one — Docling's title is a best-effort guess and
// must never clobber a title the caller explicitly supplied. Distinct from
// Update: this is the pipeline recording what it learned, not a user
// editing descriptive metadata.
func (s *DocumentService) UpdateAfterParse(
	ctx context.Context,
	id uuid.UUID,
	pageCount int,
	doclingTitle string,
) error {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	doc.PageCount = pageCount
	if doc.Title == "" {
		doc.Title = doclingTitle
	}
	doc.ModifiedAt = time.Now()

	return s.repo.Update(ctx, doc)
}
