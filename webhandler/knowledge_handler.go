package webhandler

import (
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/service"
	knowledgeview "github.com/kairosedubf/wobsongo/view/knowledge"
	"github.com/labstack/echo/v4"
)

// AtomicKnowledgeWebHandler handles the read-only web (HTML) list page for
// atomic knowledge facts, always scoped to a single document selected via a
// document_id query param.
type AtomicKnowledgeWebHandler struct {
	svc         *service.AtomicKnowledgeService
	documentSvc *service.DocumentService
}

// NewAtomicKnowledgeWebHandler constructs an AtomicKnowledgeWebHandler.
func NewAtomicKnowledgeWebHandler(
	svc *service.AtomicKnowledgeService,
	documentSvc *service.DocumentService,
) *AtomicKnowledgeWebHandler {
	return &AtomicKnowledgeWebHandler{svc: svc, documentSvc: documentSvc}
}

// listPage renders the paginated knowledge list for the document selected via
// the document_id query param. With no (or an invalid) document_id, it
// renders the empty "select a document" state rather than erroring —
// knowledge facts are deliberately never listed generically across every
// document.
func (h *AtomicKnowledgeWebHandler) listPage(c echo.Context) error {
	ctx := c.Request().Context()

	// Capped at 100 (dto.PaginationDTO's max) — acceptable for a filter
	// dropdown today since there's no search-within-dropdown pattern yet in
	// this app; revisit if the document count grows past that.
	docs, err := h.documentSvc.List(ctx, &dto.PaginationDTO{PerPage: 100})
	if err != nil {
		return err
	}

	var pagination dto.PaginationDTO
	if err := c.Bind(&pagination); err != nil || pagination.Page < 1 {
		pagination.Page = 1
	}
	if pagination.PerPage < 1 {
		pagination.PerPage = 20
	}

	documentIDParam := c.QueryParam("document_id")
	var results dto.PaginationResults[model.AtomicKnowledge]
	hasSelection := false
	if documentIDParam != "" {
		if docID, parseErr := uuid.Parse(documentIDParam); parseErr == nil {
			hasSelection = true
			r, listErr := h.svc.List(ctx, docID, &pagination)
			if listErr != nil {
				return listErr
			}
			results = *r
		}
	}

	layoutData := buildAppLayout(c, "Knowledge", "")
	return knowledgeview.List(knowledgeview.ListPageData{
		AppLayoutData:      layoutData,
		Results:            results,
		Documents:          docs.Items,
		SelectedDocumentID: documentIDParam,
		HasSelection:       hasSelection,
	}).Render(ctx, c.Response())
}
