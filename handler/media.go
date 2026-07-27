package handler

import (
	"net/http"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/kairosedubf/wobsongo/validation"
	"github.com/labstack/echo/v4"
)

// MediaHandler handles media-related HTTP requests.
type MediaHandler struct {
	service *service.MediaService
}

// NewMediaHandler creates a new instance of MediaHandler.
func NewMediaHandler(mediaSvc *service.MediaService) *MediaHandler {
	return &MediaHandler{service: mediaSvc}
}

// @Summary		Get presigned POST URL for media upload
// @Description	Generate a presigned POST URL for uploading media files.
// @Tags		media
// @Accept		json
// @Produce		json
// @Param		intent			query		string	true	"Media upload intent"
// @Param		filename		query		string	true	"Filename to be uploaded"
// @Param		content_type	query		string	true	"Content type of the file"
// @Success		200				{object}	model.APIResponse{data=model.POSTUploadPolicy}
// @Failure		404				{object}	model.APIResponse{error=string}
// @Failure		409				{object}	model.APIResponse{error=string}
// @Failure		422				{object}	model.APIResponse{error=string}
// @Router		/api/v1/media/upload [get]
func (h *MediaHandler) getPresignedPOSTURL(c echo.Context) error {
	form := new(dto.GetPresignedURLDTO)
	if err := c.Bind(form); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidQueryParams,
		}
	}
	if err := c.Validate(form); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	if !data.IsValidMediaUploadIntent(form.Intent) {
		return &model.APIError{
			Code:   http.StatusUnprocessableEntity,
			Public: "invalid media upload intent: " + form.Intent,
		}
	}
	if !validation.ValidateFilename(form.Filename) {
		return &model.APIError{
			Code:   http.StatusUnprocessableEntity,
			Public: "invalid filename format: " + form.Filename,
		}
	}

	presigned, err := h.service.GetPresignedPOSTURL(
		c.Request().Context(),
		data.MediaUploadIntent(form.Intent),
		form.Filename,
		form.ContentType,
	)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{
			Code:     status,
			Internal: err,
			Public:   message + ": filename " + form.Filename,
		}
	}

	return writeJSON(c, http.StatusOK, presigned)
}

// @Summary		Get presigned GET URL for media access
// @Description	Generate a presigned GET URL for accessing uploaded media files by S3 key.
// @Tags		media
// @Accept		json
// @Produce		json
// @Param		s3_key	query		string	true	"S3 object key (e.g., documents/abc123.pdf)"
// @Param		ttl		query		int		false	"TTL in seconds (default: 900, max: 86400)"	default(900)
// @Success		200		{object}	model.APIResponse{data=dto.PresignedURL}
// @Failure		400		{object}	model.APIResponse{error=string}	"Invalid S3 key"
// @Failure		422		{object}	model.APIResponse{error=string}	"Validation error"
// @Failure		500		{object}	model.APIResponse{error=string}	"Internal server error"
// @Router		/api/v1/media/presigned-url [get]
func (h *MediaHandler) getPresignedGETURL(c echo.Context) error {
	form := new(dto.GetPresignedGETURLDTO)
	if err := c.Bind(form); err != nil {
		return &model.APIError{
			Code:     http.StatusBadRequest,
			Internal: err,
			Public:   msgInvalidQueryParams,
		}
	}
	if err := c.Validate(form); err != nil {
		return &model.APIError{
			Code:     http.StatusUnprocessableEntity,
			Internal: err,
			Public:   msgValidationFailed,
		}
	}

	presignedURL, err := h.service.GetPresignedGETURL(c.Request().Context(), form.S3Key, form.TTL)
	if err != nil {
		status, message := mapDataErrorToHTTP(err)
		return &model.APIError{
			Code:     status,
			Internal: err,
			Public:   message + ": s3 key " + form.S3Key,
		}
	}

	ttl := form.TTL
	if ttl == 0 {
		ttl = 900
	}

	return writeJSON(c, http.StatusOK, &dto.PresignedURL{
		PresignedURL: presignedURL,
		TTL:          ttl,
	})
}
