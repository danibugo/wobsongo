// Package webhandler_test provides HTTP-layer tests for webhandler's HTML
// routes. Deliberately external (black-box) since the handler methods are
// unexported — the only entry point is webhandler.RegisterWebRoutes.
package webhandler_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	authpkg "github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/handler"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/testhelpers"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newTestConfig returns a config safe for webhandler HTTP tests: Env must be
// set explicitly since Config.IsProduction() treats an empty Env as
// production too, and IsTesting() (used by loginRateLimit to no-op the rate
// limiter) requires it.
func newTestConfig() *config.Config {
	return &config.Config{
		Env:            config.TestingEnv,
		JWTSecret:      "webhandler-test-secret",
		JWTExpiryHours: 2,
	}
}

func newTestJWTAuth(cfg *config.Config) *authpkg.Auth {
	return authpkg.New(cfg.JWTSecret, cfg.JWTExpiryHours)
}

// newWebRepos builds a WebRepos with only the given dependencies set. The
// remaining fields (ChunkRepo, KnowledgeRepo, MediaProvider, Embedder,
// ClaimAnalyzer, ClaimJudge) stay nil — safe, since every service.New*
// constructor RegisterWebRoutes calls is a trivial struct literal, and the
// routes that would actually use those nil deps (/chunks, /evidence,
// /knowledge, /retrieval, /check) are out of scope for these tests.
func newWebRepos(
	userRepo data.UserRepoer,
	documentRepo data.DocumentRepoer,
	rawStore data.RawObjectStore,
) *webhandler.WebRepos {
	return &webhandler.WebRepos{
		UserRepo:     userRepo,
		DocumentRepo: documentRepo,
		RawStore:     rawStore,
	}
}

// newWebHandlerTestApp builds an *echo.Echo wired the same way core.NewApp
// wires the HTML route group, minus CSRF — webhandler/context.go's
// csrfToken() already anticipates running without it in tests.
func newWebHandlerTestApp(
	repos *webhandler.WebRepos,
	jwtAuth *authpkg.Auth,
	cfg *config.Config,
) *echo.Echo {
	e := testhelpers.NewEcho()
	handler.UseCustomErrorHandler(e)
	g := e.Group("",
		handler.JWTFromCookieMiddleware(webhandler.AuthCookieName),
		handler.JWTParserMiddleware(jwtAuth),
	)
	webhandler.RegisterWebRoutes(g, repos, jwtAuth, cfg)
	return e
}

// mintAuthCookie mints a real JWT for payload and wraps it as the auth cookie.
func mintAuthCookie(t *testing.T, jwtAuth *authpkg.Auth, payload *authpkg.JWTPayload) *http.Cookie {
	t.Helper()
	tokens, err := jwtAuth.GenerateJWTPair(payload)
	if err != nil {
		t.Fatalf("failed to mint JWT: %v", err)
	}
	return &http.Cookie{Name: webhandler.AuthCookieName, Value: tokens.AccessToken}
}

// newAuthedRequest builds a request with a default authenticated user's
// auth cookie attached, for driving requests through protected routes.
func newAuthedRequest(
	t *testing.T,
	jwtAuth *authpkg.Auth,
	method, target string,
	body io.Reader,
) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	cookie := mintAuthCookie(t, jwtAuth, &authpkg.JWTPayload{
		ID:    uuid.New(),
		Name:  "Test User",
		Email: "test@example.com",
		Role:  model.RoleUser,
	})
	req.AddCookie(cookie)
	return req
}

// stubRawObjectStore is a hand-rolled data.RawObjectStore for testing
// without real object storage — no generated mock exists for this
// interface. Mirrors the shape of worker/parse_document_test.go's
// stubRawStore.
type stubRawObjectStore struct {
	putObjectFunc func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	getObjectFunc func(ctx context.Context, key string) (io.ReadCloser, error)
}

func (s *stubRawObjectStore) PutObject(
	ctx context.Context, key string, r io.Reader, size int64, contentType string,
) error {
	if s.putObjectFunc != nil {
		return s.putObjectFunc(ctx, key, r, size, contentType)
	}
	return nil
}

func (s *stubRawObjectStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.getObjectFunc != nil {
		return s.getObjectFunc(ctx, key)
	}
	return nil, nil
}

// newMultipartUploadRequest builds a multipart/form-data POST request for
// createPost tests. filename == "" skips the file part entirely, covering
// the "no file selected" branch.
func newMultipartUploadRequest(
	t *testing.T,
	target string,
	fields map[string]string,
	fileFieldName, filename string,
	fileContent []byte,
) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if filename != "" {
		fw, err := w.CreateFormFile(fileFieldName, filename)
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatalf("failed to write file content: %v", err)
		}
	}

	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("failed to write field %q: %v", k, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	return req
}
