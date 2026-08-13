package webhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newAuthTestApp builds an *echo.Echo wired with only a UserRepo, for
// auth_handler tests — DocumentRepo/RawStore are irrelevant here.
func newAuthTestApp(userRepo *mockrepo.UserRepoerMock) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := newWebRepos(userRepo, nil, nil)
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func formRequest(target string, values url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	return req
}

func TestRegisterPage(t *testing.T) {
	t.Run("unauthenticated renders form", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Create account") {
			t.Error("expected register form to be rendered")
		}
	})

	t.Run("already authenticated redirects home", func(t *testing.T) {
		e, jwtAuth := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/register", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Errorf("Location = %q, want %q", got, "/")
		}
	})
}

func TestRegisterPost(t *testing.T) {
	t.Run("missing name rerenders with error", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := formRequest("/register", url.Values{
			"name": {""}, "email": {"a@example.com"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Name is required.") {
			t.Errorf("expected name-required error, got body: %s", rec.Body.String())
		}
	})

	t.Run("invalid email rerenders with error", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := formRequest("/register", url.Values{
			"name": {"Ada"}, "email": {"not-an-email"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Please enter a valid email address.") {
			t.Errorf("expected invalid-email error, got body: %s", rec.Body.String())
		}
	})

	t.Run("weak password rerenders with error", func(t *testing.T) {
		tests := []struct {
			name     string
			password string
		}{
			{"too short", "short1!"},
			{"only lowercase letters", "alllowercase"},
			{"only digits", "12345678"},
			{"only symbols", "!!!!!!!!"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

				req := formRequest("/register", url.Values{
					"name": {"Ada"}, "email": {"ada@example.com"}, "password": {tt.password},
				})
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d", rec.Code)
				}
				want := "Password must be at least 8 characters and include at least two types: letters, numbers, or symbols."
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("expected weak-password error, got body: %s", rec.Body.String())
				}
			})
		}
	})

	t.Run("password with exactly two character classes and 8 chars passes validation", func(t *testing.T) {
		// "abcd1234" is exactly 8 chars with exactly 2 character classes
		// (letters + digits) — the inclusive boundary of passwordValid's
		// policy. Reaching the redirect (rather than the weak-password
		// re-render) proves the boundary is inclusive.
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.CreateFunc = func(_ context.Context, _ *model.User) error { return nil }
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/register", url.Values{
			"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected password to pass validation and redirect (302), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("service conflict rerenders with error", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.CreateFunc = func(_ context.Context, _ *model.User) error { return data.ErrConflict }
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/register", url.Values{
			"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "An account with this email already exists.") {
			t.Errorf("expected conflict error, got body: %s", rec.Body.String())
		}
	})

	t.Run("generic service error rerenders with generic message", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.CreateFunc = func(_ context.Context, _ *model.User) error { return data.ErrInternal }
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/register", url.Values{
			"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Something went wrong. Please try again.") {
			t.Errorf("expected generic error, got body: %s", rec.Body.String())
		}
	})

	t.Run("success sets cookie and redirects", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.CreateFunc = func(_ context.Context, _ *model.User) error { return nil }
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/register", url.Values{
			"name": {"Ada"}, "email": {"ada@example.com"}, "password": {"abcd1234"},
		})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Errorf("Location = %q, want %q", got, "/")
		}
		assertAuthCookieSet(t, rec)
	})
}

func TestLoginPage(t *testing.T) {
	t.Run("default renders form without error", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Sign in to continue") {
			t.Error("expected login form to be rendered")
		}
		if strings.Contains(rec.Body.String(), "Incorrect email or password.") {
			t.Error("did not expect an error message on the default login page")
		}
	})

	t.Run("error query param shows error message", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login?error=1", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Incorrect email or password. Try again.") {
			t.Errorf("expected incorrect-credentials message, got body: %s", rec.Body.String())
		}
	})

	t.Run("already authenticated redirects home", func(t *testing.T) {
		e, jwtAuth := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/login", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Errorf("Location = %q, want %q", got, "/")
		}
	})
}

func TestLoginPost(t *testing.T) {
	const password = "abcd1234"
	hash, err := auth.HashPassword(password, auth.WithMinCost())
	if err != nil {
		t.Fatalf("failed to hash password fixture: %v", err)
	}

	t.Run("success sets cookie and redirects", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.GetByEmailFunc = func(_ context.Context, _ string) (*model.User, error) {
			return &model.User{Email: "ada@example.com", PasswordHash: hash, Role: model.RoleUser}, nil
		}
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/login", url.Values{"email": {"ada@example.com"}, "password": {password}})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Location"); got != "/" {
			t.Errorf("Location = %q, want %q", got, "/")
		}
		assertAuthCookieSet(t, rec)
	})

	t.Run("wrong password redirects to login with error and no cookie", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.GetByEmailFunc = func(_ context.Context, _ string) (*model.User, error) {
			return &model.User{Email: "ada@example.com", PasswordHash: hash, Role: model.RoleUser}, nil
		}
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/login", url.Values{"email": {"ada@example.com"}, "password": {"wrong-password"}})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assertLoginFailure(t, rec)
	})

	t.Run("unknown email redirects to login with error and no cookie", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.GetByEmailFunc = func(_ context.Context, _ string) (*model.User, error) {
			return nil, data.ErrNotFound
		}
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/login", url.Values{"email": {"ghost@example.com"}, "password": {password}})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assertLoginFailure(t, rec)
	})

	t.Run("generic repo error still redirects generically, never 500", func(t *testing.T) {
		userRepo := &mockrepo.UserRepoerMock{}
		userRepo.GetByEmailFunc = func(_ context.Context, _ string) (*model.User, error) {
			return nil, data.ErrInternal
		}
		e, _ := newAuthTestApp(userRepo)

		req := formRequest("/login", url.Values{"email": {"ada@example.com"}, "password": {password}})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assertLoginFailure(t, rec)
	})
}

func assertLoginFailure(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/login?error=1" {
		t.Errorf("Location = %q, want %q", got, "/login?error=1")
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == webhandler.AuthCookieName {
			t.Errorf("expected no auth cookie to be set, got one with value %q", c.Value)
		}
	}
}

func assertAuthCookieSet(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name != webhandler.AuthCookieName {
			continue
		}
		if c.Value == "" {
			t.Error("expected auth cookie to have a non-empty value")
		}
		if !c.HttpOnly {
			t.Error("expected auth cookie to be HttpOnly")
		}
		if c.SameSite != http.SameSiteLaxMode {
			t.Errorf("SameSite = %v, want Lax", c.SameSite)
		}
		if c.Secure {
			t.Error("expected auth cookie not to be Secure under TestingEnv")
		}
		wantExpiry := time.Now().Add(2 * time.Hour)
		if c.Expires.Before(wantExpiry.Add(-time.Minute)) || c.Expires.After(wantExpiry.Add(time.Minute)) {
			t.Errorf("Expires = %v, want approximately %v", c.Expires, wantExpiry)
		}
		return
	}
	t.Fatal("expected auth cookie to be set, found none")
}

func TestLogoutPost(t *testing.T) {
	t.Run("no auth cookie still clears and redirects", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/logout", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/login" {
			t.Errorf("Location = %q, want %q", got, "/login")
		}
	})

	t.Run("with auth cookie clears cookie and redirects", func(t *testing.T) {
		e, jwtAuth := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodPost, "/logout", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/login" {
			t.Errorf("Location = %q, want %q", got, "/login")
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name != webhandler.AuthCookieName {
				continue
			}
			if c.Value != "" {
				t.Errorf("expected cleared cookie value, got %q", c.Value)
			}
			if c.MaxAge != -1 {
				t.Errorf("MaxAge = %d, want -1", c.MaxAge)
			}
			return
		}
		t.Fatal("expected a Set-Cookie clearing the auth cookie, found none")
	})
}
