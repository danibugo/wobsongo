package webhandler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/service"
	evidenceview "github.com/kairosedubf/wobsongo/view/evidence"
	"github.com/labstack/echo/v4"
)

// EvidenceWebHandler renders a standalone, chrome-free page showing exactly
// one chunk's location (page + bounding box) in its source PDF — meant as a
// link target (e.g. from a claim check's citations), not a page navigated to
// from within the dashboard.
type EvidenceWebHandler struct {
	chunkSvc    *service.DocumentChunkService
	documentSvc *service.DocumentService
	mediaSvc    *service.MediaService
}

// NewEvidenceWebHandler constructs an EvidenceWebHandler.
func NewEvidenceWebHandler(
	chunkSvc *service.DocumentChunkService,
	documentSvc *service.DocumentService,
	mediaSvc *service.MediaService,
) *EvidenceWebHandler {
	return &EvidenceWebHandler{chunkSvc: chunkSvc, documentSvc: documentSvc, mediaSvc: mediaSvc}
}

// viewPage resolves the document_id/chunk_id query params and renders the
// pdf-bbox-viewer positioned on that chunk's page/bbox.
func (h *EvidenceWebHandler) viewPage(c echo.Context) error {
	ctx := c.Request().Context()

	docID, err := uuid.Parse(c.QueryParam("document_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid document_id.")
	}
	chunkID, err := uuid.Parse(c.QueryParam("chunk_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid chunk_id.")
	}

	chunk, err := h.chunkSvc.GetByID(ctx, chunkID)
	if err != nil {
		return err
	}
	if chunk.DocumentID != docID {
		return echo.NewHTTPError(http.StatusNotFound, "Chunk does not belong to that document.")
	}

	doc, err := h.documentSvc.GetByID(ctx, docID)
	if err != nil {
		return err
	}
	if doc.Filetype != pdfContentType {
		return echo.NewHTTPError(
			http.StatusUnprocessableEntity,
			"Evidence viewing is only available for PDF documents.",
		)
	}

	pdfURL, err := h.mediaSvc.GetPresignedGETURL(ctx, string(doc.FileURL), 0)
	if err != nil {
		return err
	}

	title := doc.Title
	if title == "" {
		title = doc.Filename
	}

	return evidenceview.View(evidenceview.ViewPageData{
		DocumentTitle: title,
		Page:          chunk.Page,
		PDFURL:        pdfURL,
		BoundingBox:   chunk.BoundingBox,
	}).Render(ctx, c.Response())
}
