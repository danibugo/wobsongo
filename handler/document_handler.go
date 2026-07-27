package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/labstack/echo/v4"
)

const (
	msgInvalidDocumentID  = "invalid document id"
	msgValidationFailed   = "validation failed"
	msgInvalidQueryParams = "invalid query parameters"
)

// DocumentHandler handles HTTP requests related to documents.
type DocumentHandler struct {
	service *service.DocumentService
}

// NewDocumentHandler creates a new handler.
func NewDocumentHandler(service *service.DocumentService) *DocumentHandler {
	return &DocumentHandler{
		service: service,
	}
}

// @Summary		Create a document
// @Description	Ingests a new document into the knowledge base.
// @Tags		documents
// @Accept		json
// @Produce		json
// @Param		form	body		dto.CreateDocumentDTO	true	"Create Document Form"
// @Success		201		{object}	model.APIResponse{data=model.Document}
// @Failure		400		{object}	model.APIResponse{error=string}
// @Failure		422		{object}	model.APIResponse{error=string}
// @Failure		500		{object}	model.APIResponse{error=string}
// @Router		/api/v1/documents [post]
func (h *DocumentHandler) createDocumentHandler(c echo.Context) error {
	req := new(dto.CreateDocumentDTO)
	if err := c.Bind(req); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   "invalid request body",
		}
	}
	if err := c.Validate(req); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	doc, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return writeJSON(c, http.StatusCreated, doc)
}

// @Summary		Get a document by ID
// @Description	Retrieves a single document by its ID.
// @Tags		documents
// @Produce		json
// @Param		id		path		string	true	"Document ID"	format(uuid)
// @Success		200		{object}	model.APIResponse{data=model.Document}
// @Failure		400		{object}	model.APIResponse{error=string}
// @Failure		404		{object}	model.APIResponse{error=string}
// @Failure		500		{object}	model.APIResponse{error=string}
// @Router		/api/v1/documents/{id} [get]
func (h *DocumentHandler) getDocumentHandler(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidDocumentID,
		}
	}

	doc, err := h.service.GetByID(c.Request().Context(), id)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return writeJSON(c, http.StatusOK, doc)
}

// @Summary		List documents
// @Description	Retrieves a paginated list of documents.
// @Tags		documents
// @Produce		json
// @Param		page		query		int	false	"Page number"			minimum(1)
// @Param		per_page	query		int	false	"Items per page"		minimum(1)	maximum(100)
// @Success		200			{object}	model.APIResponse{data=dto.PaginationResults[model.Document]}
// @Failure		400			{object}	model.APIResponse{error=string}
// @Failure		422			{object}	model.APIResponse{error=string}
// @Failure		500			{object}	model.APIResponse{error=string}
// @Router		/api/v1/documents [get]
func (h *DocumentHandler) listDocumentsHandler(c echo.Context) error {
	pagination := new(dto.PaginationDTO)
	if err := c.Bind(pagination); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidQueryParams,
		}
	}
	if err := c.Validate(pagination); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	results, err := h.service.List(c.Request().Context(), pagination)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return writeJSON(c, http.StatusOK, results)
}

// @Summary		Update a document
// @Description	Updates a document's descriptive metadata.
// @Tags		documents
// @Accept		json
// @Produce		json
// @Param		id		path		string					true	"Document ID"	format(uuid)
// @Param		form	body		dto.UpdateDocumentDTO	true	"Update Document Form"
// @Success		200		{object}	model.APIResponse{data=model.Document}
// @Failure		400		{object}	model.APIResponse{error=string}
// @Failure		403		{object}	model.APIResponse{error=string}
// @Failure		404		{object}	model.APIResponse{error=string}
// @Failure		409		{object}	model.APIResponse{error=string}
// @Failure		422		{object}	model.APIResponse{error=string}
// @Failure		500		{object}	model.APIResponse{error=string}
// @Router		/api/v1/documents/{id} [put]
func (h *DocumentHandler) updateDocumentHandler(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidDocumentID,
		}
	}

	req := new(dto.UpdateDocumentDTO)
	if err := c.Bind(req); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   "invalid request body",
		}
	}
	if err := c.Validate(req); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	doc, err := h.service.Update(c.Request().Context(), id, req)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return writeJSON(c, http.StatusOK, doc)
}

// @Summary		Delete a document
// @Description	Deletes a document by its ID.
// @Tags		documents
// @Param		id	path	string	true	"Document ID"	format(uuid)
// @Success		204
// @Failure		400	{object}	model.APIResponse{error=string}
// @Failure		403	{object}	model.APIResponse{error=string}
// @Failure		404	{object}	model.APIResponse{error=string}
// @Failure		500	{object}	model.APIResponse{error=string}
// @Router		/api/v1/documents/{id} [delete]
func (h *DocumentHandler) deleteDocumentHandler(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidDocumentID,
		}
	}

	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{Code: status, Internal: err, Public: message}
	}

	return c.NoContent(http.StatusNoContent)
}
