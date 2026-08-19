package webhandler

import (
	"context"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/service"
	retrievalview "github.com/kairosedubf/wobsongo/view/retrieval"
	"github.com/labstack/echo/v4"
)

// retrievalResultLimit caps how many fused hybrid-search results are shown —
// the CLI's ragDefaultLimit is 10; the web page has room to show more.
const retrievalResultLimit = 20

// RetrievalWebHandler handles the read-only web (HTML) search page over the
// knowledge base — the web equivalent of the `rag` CLI command.
type RetrievalWebHandler struct {
	ragSvc      *service.RAGService
	documentSvc *service.DocumentService
	mediaSvc    *service.MediaService
}

// NewRetrievalWebHandler constructs a RetrievalWebHandler.
func NewRetrievalWebHandler(
	ragSvc *service.RAGService,
	documentSvc *service.DocumentService,
	mediaSvc *service.MediaService,
) *RetrievalWebHandler {
	return &RetrievalWebHandler{ragSvc: ragSvc, documentSvc: documentSvc, mediaSvc: mediaSvc}
}

// retrievalDocInfo is the per-document display data resolved once per
// distinct DocumentID in a result set, not once per row.
type retrievalDocInfo struct {
	title  string
	pdfURL string
}

// listPage renders search results for the query in the q query param. With
// no query, it renders the empty "enter a query" state.
func (h *RetrievalWebHandler) listPage(c echo.Context) error {
	ctx := c.Request().Context()
	query := c.QueryParam("q")

	var rows []retrievalview.Row
	if query != "" {
		results, err := h.ragSvc.Search(ctx, query, retrievalResultLimit)
		if err != nil {
			return err
		}

		docCache := make(map[uuid.UUID]retrievalDocInfo, len(results))
		rows = make([]retrievalview.Row, 0, len(results))
		for i := range results {
			r := &results[i]
			info, err := h.resolveDoc(ctx, docCache, r.DocumentID)
			if err != nil {
				return err
			}
			rows = append(rows, retrievalview.Row{
				RAGResult:     *r,
				DocumentTitle: info.title,
				PDFURL:        info.pdfURL,
			})
		}
	}

	layoutData := buildAppLayout(c, "Retrieval")
	return retrievalview.List(retrievalview.ListPageData{
		AppLayoutData: layoutData,
		Query:         query,
		Results:       rows,
	}).Render(ctx, c.Response())
}

// resolveDoc resolves a result's source document's display title and (for
// PDFs) a presigned GET URL, caching per DocumentID so a result set spanning
// many hits from the same document only resolves it once.
func (h *RetrievalWebHandler) resolveDoc(
	ctx context.Context,
	cache map[uuid.UUID]retrievalDocInfo,
	docID uuid.UUID,
) (retrievalDocInfo, error) {
	if info, ok := cache[docID]; ok {
		return info, nil
	}

	doc, err := h.documentSvc.GetByID(ctx, docID)
	if err != nil {
		return retrievalDocInfo{}, err
	}

	info := retrievalDocInfo{title: doc.Title}
	if info.title == "" {
		info.title = doc.Filename
	}
	if doc.Filetype == pdfContentType {
		url, err := h.mediaSvc.GetPresignedGETURL(ctx, string(doc.FileURL), 0)
		if err != nil {
			return retrievalDocInfo{}, err
		}
		info.pdfURL = url
	}

	cache[docID] = info
	return info, nil
}
