package handler

import (
	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/labstack/echo/v4"
)

// writeJSON writes a successful JSON response wrapped in the standard
// model.APIResponse envelope, attaching the request ID set by requestIDMiddleware.
func writeJSON(c echo.Context, status int, data any) error {
	reqID, ok := c.Get(contextRequestID).(string)
	if !ok {
		reqID = uuid.New().String()
	}

	requestID, _ := uuid.Parse(reqID)

	return c.JSON(status, model.APIResponse{
		Status:    status,
		Data:      data,
		RequestID: requestID,
	})
}
