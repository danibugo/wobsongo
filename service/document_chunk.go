package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/model"
)

// DocumentChunkService defines a set of available methods related to
// document chunk operations.
type DocumentChunkService struct {
	repo data.DocumentChunkRepoer
}

// NewDocumentChunkService creates a new DocumentChunkService.
func NewDocumentChunkService(repo data.DocumentChunkRepoer) *DocumentChunkService {
	return &DocumentChunkService{repo: repo}
}

// ListByPage retrieves a document's chunks on exactly one page, ordered by
// SequenceNumber.
func (s *DocumentChunkService) ListByPage(
	ctx context.Context,
	documentID uuid.UUID,
	page int,
) ([]model.DocumentChunk, error) {
	return s.repo.ListByDocumentIDAndPage(ctx, documentID, page)
}
