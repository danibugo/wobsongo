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

// List retrieves a paginated page of chunks for a single document.
func (s *DocumentChunkService) List(
	ctx context.Context,
	documentID uuid.UUID,
	pagination *dto.PaginationDTO,
) (*dto.PaginationResults[model.DocumentChunk], error) {
	return s.repo.PaginateByDocumentID(ctx, documentID, pagination)
}
