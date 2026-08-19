package webhandler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
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

// listPage renders the chunk list for exactly one PDF page of the document
// selected via the document_id query param, with a pagination nav mapping
// 1:1 onto the document's page count. With no (or an invalid/not-found)
// document_id, it renders the empty "select a document" state rather than
// erroring — chunks are deliberately never listed generically across every
// document.
func (h *DocumentChunkWebHandler) listPage(c echo.Context) error {
	ctx := c.Request().Context()

	// Capped at 100 (dto.PaginationDTO's max) — acceptable for a filter
	// dropdown today since there's no search-within-dropdown pattern yet in
	// this app; revisit if the document count grows past that.
	docs, err := h.documentSvc.List(ctx, &dto.PaginationDTO{PerPage: 100})
	if err != nil {
		return err
	}

	var pageParam dto.PaginationDTO
	_ = c.Bind(&pageParam)
	requestedPage := int(pageParam.GetPage())

	documentIDParam := c.QueryParam("document_id")
	var chunks []model.DocumentChunk
	var pdfURL string
	currentPage := 1
	totalPages := 0
	hasSelection := false
	if documentIDParam != "" {
		if docID, parseErr := uuid.Parse(documentIDParam); parseErr == nil {
			doc, getErr := h.documentSvc.GetByID(ctx, docID)
			if getErr != nil && !errors.Is(getErr, data.ErrNotFound) {
				return getErr
			}
			if doc != nil {
				hasSelection = true
				totalPages = doc.PageCount

				currentPage = max(requestedPage, 1)
				if totalPages > 0 && currentPage > totalPages {
					currentPage = totalPages
				}

				if totalPages > 0 {
					chunks, err = h.svc.ListByPage(ctx, docID, currentPage)
					if err != nil {
						return err
					}
				}

				if doc.Filetype == pdfContentType {
					url, presignErr := h.mediaSvc.GetPresignedGETURL(ctx, string(doc.FileURL), 0)
					if presignErr != nil {
						return presignErr
					}
					pdfURL = url
				}
			}
		}
	}

	layoutData := buildAppLayout(c, "Chunks")
	return chunkview.List(chunkview.ListPageData{
		AppLayoutData:      layoutData,
		Chunks:             chunks,
		CurrentPage:        currentPage,
		TotalPages:         totalPages,
		Documents:          docs.Items,
		SelectedDocumentID: documentIDParam,
		HasSelection:       hasSelection,
		PDFURL:             pdfURL,
	}).Render(ctx, c.Response())
}
