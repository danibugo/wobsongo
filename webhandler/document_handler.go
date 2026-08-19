package webhandler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/service"
	docview "github.com/kairosedubf/wobsongo/view/documents"
	"github.com/labstack/echo/v4"
)

// allowedDocumentExtensions is the set of file extensions accepted on upload,
// matching the CLI whitelist in cmd/document_insert.go.
var allowedDocumentExtensions = map[string]bool{
	".pdf":  true,
	".docx": true,
	".doc":  true,
	".rtf":  true,
	".html": true,
	".md":   true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".avif": true,
}

const maxUploadBytes = 50 << 20 // 50 MB

// DocumentWebHandler handles the web (HTML) CRUD pages for documents.
type DocumentWebHandler struct {
	svc      *service.DocumentService
	rawStore data.RawObjectStore
}

// NewDocumentWebHandler constructs a DocumentWebHandler.
func NewDocumentWebHandler(
	svc *service.DocumentService,
	rawStore data.RawObjectStore,
) *DocumentWebHandler {
	return &DocumentWebHandler{svc: svc, rawStore: rawStore}
}

// listPage renders the paginated document list.
func (h *DocumentWebHandler) listPage(c echo.Context) error {
	var pagination dto.PaginationDTO
	if err := c.Bind(&pagination); err != nil || pagination.Page < 1 {
		pagination.Page = 1
	}
	if pagination.PerPage < 1 {
		pagination.PerPage = 20
	}

	results, err := h.svc.List(c.Request().Context(), &pagination)
	if err != nil {
		return err
	}

	layoutData := buildAppLayout(c, "Documents")
	return docview.List(docview.ListPageData{
		AppLayoutData: layoutData,
		Results:       *results,
	}).Render(c.Request().Context(), c.Response())
}

// newPage renders the create/upload form.
func (h *DocumentWebHandler) newPage(c echo.Context) error {
	layoutData := buildAppLayout(c, "New Document")
	return docview.Form(docview.FormPageData{
		AppLayoutData: layoutData,
	}).Render(c.Request().Context(), c.Response())
}

// createPost handles file upload and document creation.
func (h *DocumentWebHandler) createPost(c echo.Context) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxUploadBytes)

	renderErr := func(msg string) error {
		layoutData := buildAppLayout(c, "New Document")
		return docview.Form(docview.FormPageData{
			AppLayoutData: layoutData,
			Error:         msg,
		}).Render(c.Request().Context(), c.Response())
	}

	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return renderErr("Please select a file to upload.")
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedDocumentExtensions[ext] {
		return renderErr(fmt.Sprintf("File type %q is not supported.", ext))
	}

	// Read file into buffer while computing SHA256 in one pass.
	var buf bytes.Buffer
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)
	size, err := io.Copy(&buf, tee)
	if err != nil {
		return renderErr("Failed to read uploaded file.")
	}
	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	s3Key := fmt.Sprintf("documents/%s%s", sha256Hex, ext)

	ctx := c.Request().Context()
	if err := h.rawStore.PutObject(ctx, s3Key, &buf, size, contentType); err != nil {
		return renderErr("Failed to upload file. Please try again.")
	}

	title := strings.TrimSpace(c.FormValue("title"))
	publisher := strings.TrimSpace(c.FormValue("publisher"))
	language := c.FormValue("language")
	if language != "en" && language != "fr" {
		return renderErr("Please select a language.")
	}

	var publicationYear int
	if y := strings.TrimSpace(c.FormValue("year")); y != "" {
		publicationYear, _ = strconv.Atoi(y)
	}

	_, err = h.svc.Create(ctx, &dto.CreateDocumentDTO{
		SHA256:          sha256Hex,
		FileKey:         s3Key,
		Title:           title,
		Filename:        header.Filename,
		Filetype:        contentType,
		Filesize:        size,
		PageCount:       0, // backfilled by Docling after parsing
		PublisherName:   publisher,
		PublicationYear: publicationYear,
		Language:        language,
	})
	if err != nil {
		return renderErr("Failed to save document: " + err.Error())
	}

	return c.Redirect(http.StatusFound, "/documents")
}

// editPage renders the metadata edit form for an existing document.
func (h *DocumentWebHandler) editPage(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Document not found.")
	}

	doc, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return err
	}

	layoutData := buildAppLayout(c, "Edit Document")
	return docview.Form(docview.FormPageData{
		AppLayoutData: layoutData,
		Document:      doc,
	}).Render(c.Request().Context(), c.Response())
}

// updatePost handles metadata updates for an existing document.
func (h *DocumentWebHandler) updatePost(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Document not found.")
	}

	renderErr := func(_ any, msg string) error {
		layoutData := buildAppLayout(c, "Edit Document")
		// Re-fetch document for form pre-fill.
		existing, fetchErr := h.svc.GetByID(c.Request().Context(), id)
		if fetchErr != nil {
			return fetchErr
		}
		return docview.Form(docview.FormPageData{
			AppLayoutData: layoutData,
			Document:      existing,
			Error:         msg,
		}).Render(c.Request().Context(), c.Response())
	}

	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		return renderErr(nil, "Title is required.")
	}

	publisher := strings.TrimSpace(c.FormValue("publisher"))
	var publicationYear int
	if y := strings.TrimSpace(c.FormValue("year")); y != "" {
		publicationYear, _ = strconv.Atoi(y)
	}

	_, err = h.svc.Update(c.Request().Context(), id, &dto.UpdateDocumentDTO{
		Title:           title,
		PublisherName:   publisher,
		PublicationYear: publicationYear,
	})
	if err != nil {
		return renderErr(nil, "Failed to update document: "+err.Error())
	}

	return c.Redirect(http.StatusFound, "/documents")
}

// deletePost deletes a document by ID.
func (h *DocumentWebHandler) deletePost(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Document not found.")
	}

	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return err
	}

	return c.Redirect(http.StatusFound, "/documents")
}
