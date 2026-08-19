package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
)

// AtomicKnowledgeService defines a set of available methods related to
// atomic knowledge operations.
type AtomicKnowledgeService struct {
	repo data.AtomicKnowledgeRepoer
}

// NewAtomicKnowledgeService creates a new AtomicKnowledgeService.
func NewAtomicKnowledgeService(repo data.AtomicKnowledgeRepoer) *AtomicKnowledgeService {
	return &AtomicKnowledgeService{repo: repo}
}

// List retrieves a paginated page of facts for a single document.
func (s *AtomicKnowledgeService) List(
	ctx context.Context,
	documentID uuid.UUID,
	pagination *dto.PaginationDTO,
) (*dto.PaginationResults[model.AtomicKnowledge], error) {
	return s.repo.PaginateByDocumentID(ctx, documentID, pagination)
}
