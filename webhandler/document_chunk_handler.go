package webhandler

import (
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/service"
	chunkview "github.com/kairosedubf/wobsongo/view/chunks"
	"github.com/labstack/echo/v4"
)

// pdfContentType is the MIME type documents are stored under when uploaded as
// a PDF (see allowedDocumentExtensions/mime.TypeByExtension in
// document_handler.go) — the PDF viewer pane only makes sense for these.
const pdfContentType = "application/pdf"

// DocumentChunkWebHandler handles the read-only web (HTML) list page for
// document chunks, always scoped to a single document selected via a
// document_id query param.
type DocumentChunkWebHandler struct {
	svc         *service.DocumentChunkService
	documentSvc *service.DocumentService
	mediaSvc    *service.MediaService
}

// NewDocumentChunkWebHandler constructs a DocumentChunkWebHandler.
func NewDocumentChunkWebHandler(
	svc *service.DocumentChunkService,
	documentSvc *service.DocumentService,
	mediaSvc *service.MediaService,
) *DocumentChunkWebHandler {
	return &DocumentChunkWebHandler{svc: svc, documentSvc: documentSvc, mediaSvc: mediaSvc}
}

// listPage renders the paginated chunk list for the document selected via the
// document_id query param. With no (or an invalid) document_id, it renders
// the empty "select a document" state rather than erroring — chunks are
// deliberately never listed generically across every document.
func (h *DocumentChunkWebHandler) listPage(c echo.Context) error {
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
	var results dto.PaginationResults[model.DocumentChunk]
	var pdfURL string
	hasSelection := false
	if documentIDParam != "" {
		if docID, parseErr := uuid.Parse(documentIDParam); parseErr == nil {
			hasSelection = true
			r, listErr := h.svc.List(ctx, docID, &pagination)
			if listErr != nil {
				return listErr
			}
			results = *r

			for i := range docs.Items {
				doc := &docs.Items[i]
				if doc.ID == docID && doc.Filetype == pdfContentType {
					url, presignErr := h.mediaSvc.GetPresignedGETURL(ctx, string(doc.FileURL), 0)
					if presignErr != nil {
						return presignErr
					}
					pdfURL = url
					break
				}
			}
		}
	}

	layoutData := buildAppLayout(c, "Chunks")
	return chunkview.List(chunkview.ListPageData{
		AppLayoutData:      layoutData,
		Results:            results,
		Documents:          docs.Items,
		SelectedDocumentID: documentIDParam,
		HasSelection:       hasSelection,
		PDFURL:             pdfURL,
	}).Render(ctx, c.Response())
}
