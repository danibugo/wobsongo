package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
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

// ListGroupedByPage retrieves a paginated set of a document's pages, each
// with its chunks grouped underneath, ordered by page ASC then
// SequenceNumber ASC.
func (s *DocumentChunkService) ListGroupedByPage(
	ctx context.Context,
	documentID uuid.UUID,
	pagination *dto.PaginationDTO,
) (*dto.PaginationResults[model.DocumentChunkPage], error) {
	return s.repo.PaginateGroupedByPage(ctx, documentID, pagination)
}
