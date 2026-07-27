package testhelpers

import (
	"github.com/kairosedubf/wobsongo/validation"
	"github.com/labstack/echo/v4"
)

// NewEcho builds an *echo.Echo with the project's DTO validator registered,
// for use in HTTP-layer handler tests.
func NewEcho() *echo.Echo {
	e := echo.New()
	if err := validation.Register(e); err != nil {
		panic(err)
	}
	return e
}
