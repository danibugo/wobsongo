package webhandler

import (
	errview "github.com/kairosedubf/wobsongo/view/error"
	"github.com/labstack/echo/v4"
)

const msgSomethingWentWrong = "Something went wrong. Please try again."

func renderHTMLError(c echo.Context, status int, msg string) error {
	c.Response().WriteHeader(status)
	return errview.Error(status, msg).Render(c.Request().Context(), c.Response())
}
