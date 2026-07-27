package webhandler

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"time"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/handler"
	"github.com/kairosedubf/wobsongo/service"
	authview "github.com/kairosedubf/wobsongo/view/auth"
	"github.com/labstack/echo/v4"
)

var (
	reHasLetter = regexp.MustCompile(`[a-zA-Z]`)
	reHasDigit  = regexp.MustCompile(`[0-9]`)
	reHasSymbol = regexp.MustCompile(`[^a-zA-Z0-9]`)
)

// passwordValid reports whether s meets the password policy:
// at least 8 characters, containing at least 2 distinct character types
// (letters, digits, symbols).
func passwordValid(s string) bool {
	if len(s) < 8 {
		return false
	}
	types := 0
	if reHasLetter.MatchString(s) {
		types++
	}
	if reHasDigit.MatchString(s) {
		types++
	}
	if reHasSymbol.MatchString(s) {
		types++
	}
	return types >= 2
}

// AuthCookieName is the name of the HTTP cookie carrying the access token
// for HTML routes. Exported so core/app.go can reference it when wiring the
// cookie-based JWT middleware onto the web route group.
const AuthCookieName = "auth"

// AuthHandler handles HTML authentication routes.
type AuthHandler struct {
	config *appconfig.Config
	svc    *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(svc *service.AuthService, cfg *appconfig.Config) *AuthHandler {
	return &AuthHandler{svc: svc, config: cfg}
}

// registerPage renders GET /register.
func (h *AuthHandler) registerPage(c echo.Context) error {
	if handler.ClaimsFromContext(c) != nil {
		return c.Redirect(http.StatusFound, "/")
	}
	return authview.Register(csrfToken(c), "").Render(c.Request().Context(), c.Response())
}

// registerPost handles POST /register — creates an account, sets the auth
// cookie, and redirects to / on success or re-renders the form on failure.
func (h *AuthHandler) registerPost(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.FormValue("name")
	email := c.FormValue("email")
	password := c.FormValue("password")

	if name == "" {
		return authview.Register(csrfToken(c), "Name is required.").Render(ctx, c.Response())
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return authview.Register(csrfToken(c), "Please enter a valid email address.").
			Render(ctx, c.Response())
	}
	if !passwordValid(password) {
		return authview.Register(
			csrfToken(c),
			"Password must be at least 8 characters and include at least two types: letters, numbers, or symbols.",
		).Render(ctx, c.Response())
	}

	_, tokens, err := h.svc.Register(ctx, name, email, password)
	if err != nil {
		msg := msgSomethingWentWrong
		if errors.Is(err, data.ErrConflict) {
			msg = "An account with this email already exists."
		}
		return authview.Register(csrfToken(c), msg).Render(ctx, c.Response())
	}
	h.setAuthCookie(c, tokens.AccessToken)
	return c.Redirect(http.StatusFound, "/")
}

// loginPage renders GET /login.
func (h *AuthHandler) loginPage(c echo.Context) error {
	if handler.ClaimsFromContext(c) != nil {
		return c.Redirect(http.StatusFound, "/")
	}
	var errMsg string
	if c.QueryParam("error") != "" {
		errMsg = "Incorrect email or password. Try again."
	}
	return authview.Login(csrfToken(c), errMsg).Render(c.Request().Context(), c.Response())
}

// loginPost handles POST /login — validates credentials, sets the auth
// cookie, and redirects to / on success or back to /login?error=1 on failure.
func (h *AuthHandler) loginPost(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	tokens, err := h.svc.Login(c.Request().Context(), email, password)
	if err != nil {
		return c.Redirect(http.StatusFound, "/login?error=1")
	}

	h.setAuthCookie(c, tokens.AccessToken)
	return c.Redirect(http.StatusFound, "/")
}

// logoutPost handles POST /logout — clears the auth cookie and redirects to /login.
func (h *AuthHandler) logoutPost(c echo.Context) error {
	c.SetCookie(&http.Cookie{ //nolint:gosec // Secure omitted: app supports plain HTTP in dev
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   h.config.IsProduction(),
	})
	return c.Redirect(http.StatusFound, "/login")
}

func (h *AuthHandler) setAuthCookie(c echo.Context, accessToken string) {
	c.SetCookie(&http.Cookie{ //nolint:gosec // Secure omitted: app supports plain HTTP in dev
		Name:     AuthCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(time.Duration(h.config.JWTExpiryHours) * time.Hour),
		Secure:   h.config.IsProduction(),
	})
}
