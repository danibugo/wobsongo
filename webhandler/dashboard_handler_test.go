package webhandler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/mockrepo"
)

func TestDashboardPage(t *testing.T) {
	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newAuthTestApp(&mockrepo.UserRepoerMock{})

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "/login" {
			t.Errorf("Location = %q, want %q", got, "/login")
		}
	})

	t.Run("authenticated renders welcome message", func(t *testing.T) {
		e, jwtAuth := newAuthTestApp(&mockrepo.UserRepoerMock{})

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Welcome, Test User.") {
			t.Errorf("expected welcome message, got body: %s", rec.Body.String())
		}
	})
}
