package webhandler

import (
	"strings"

	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/service"
	checkview "github.com/kairosedubf/wobsongo/view/check"
	"github.com/labstack/echo/v4"
)

// CheckWebHandler handles the web (HTML) claim-checking page — the web
// equivalent of the `claim-check` CLI command.
type CheckWebHandler struct {
	claimSvc *service.ClaimService
}

// NewCheckWebHandler constructs a CheckWebHandler.
func NewCheckWebHandler(claimSvc *service.ClaimService) *CheckWebHandler {
	return &CheckWebHandler{claimSvc: claimSvc}
}

// formPage renders the empty claim-check form.
func (h *CheckWebHandler) formPage(c echo.Context) error {
	layoutData := buildAppLayout(c, "Check")
	return checkview.Form(checkview.FormPageData{
		AppLayoutData: layoutData,
	}).Render(c.Request().Context(), c.Response())
}

// checkPost runs CheckClaim synchronously and re-renders the same page with
// the result — CheckClaim has no job/polling step, so a plain POST-then-render
// is enough (see service.ClaimService.CheckClaim).
func (h *CheckWebHandler) checkPost(c echo.Context) error {
	ctx := c.Request().Context()
	text := strings.TrimSpace(c.FormValue("text"))
	isLong := c.FormValue("mode") == "long"

	renderErr := func(msg string) error {
		layoutData := buildAppLayout(c, "Check")
		return checkview.Form(checkview.FormPageData{
			AppLayoutData: layoutData,
			Text:          text,
			IsLong:        isLong,
			Error:         msg,
		}).Render(ctx, c.Response())
	}

	if text == "" {
		return renderErr("Please enter a claim to check.")
	}

	result, err := h.claimSvc.CheckClaim(ctx, &dto.CheckClaimDTO{Text: text, IsLong: isLong})
	if err != nil {
		return renderErr("Failed to check claim: " + err.Error())
	}

	layoutData := buildAppLayout(c, "Check")
	return checkview.Form(checkview.FormPageData{
		AppLayoutData: layoutData,
		Text:          text,
		IsLong:        isLong,
		Result:        result,
	}).Render(ctx, c.Response())
}
