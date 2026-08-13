package webhandler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
)

// newCheckTestApp builds an *echo.Echo wired with a ClaimAnalyzer, ClaimJudge,
// DocumentChunkRepo, AtomicKnowledgeRepo, and Embedder — everything
// ClaimService's internal RAGService needs — for check_handler tests.
func newCheckTestApp(
	analyzer data.ClaimAnalyzer,
	judge data.ClaimJudge,
	chunkRepo *mockrepo.DocumentChunkRepoerMock,
	knowledgeRepo *mockrepo.AtomicKnowledgeRepoerMock,
	embedder data.Embedder,
) (*echo.Echo, *auth.Auth) {
	cfg := newTestConfig()
	jwtAuth := newTestJWTAuth(cfg)
	repos := &webhandler.WebRepos{
		ClaimAnalyzer: analyzer,
		ClaimJudge:    judge,
		ChunkRepo:     chunkRepo,
		KnowledgeRepo: knowledgeRepo,
		Embedder:      embedder,
	}
	return newWebHandlerTestApp(repos, jwtAuth, cfg), jwtAuth
}

func TestCheckFormPage(t *testing.T) {
	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newCheckTestApp(nil, nil, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, nil)

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/check", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("authenticated renders empty form", func(t *testing.T) {
		e, jwtAuth := newCheckTestApp(nil, nil, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, nil)

		req := newAuthedRequest(t, jwtAuth, http.MethodGet, "/check", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestCheckPost(t *testing.T) {
	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		e, _ := newCheckTestApp(nil, nil, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, nil)

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/check", nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	})

	t.Run("missing text rerenders with error", func(t *testing.T) {
		// analyzer/judge/embedder left nil: if CheckClaim is reached at all,
		// the call panics, proving the missing-text branch short-circuits
		// before ever touching ClaimService.
		e, jwtAuth := newCheckTestApp(nil, nil, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, nil)

		req := formRequest("/check", url.Values{"text": {""}})
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Please enter a claim to check.") {
			t.Errorf("expected missing-text error, got body: %s", rec.Body.String())
		}
	})

	t.Run("ClaimService error rerenders with message (swallowed, not 500)", func(t *testing.T) {
		analyzer := &stubClaimAnalyzer{err: errors.New("analyzer unavailable")}
		e, jwtAuth := newCheckTestApp(analyzer, &unreachableClaimJudge{t: t}, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, &stubEmbedder{})

		req := formRequest("/check", url.Values{"text": {"is coffee healthy?"}})
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 (swallowed error, not 500), got %d: %s", rec.Code, rec.Body.String())
		}
		// ClaimService.CheckClaim wraps the analyzer's error with its own
		// "failed to analyze claim: " prefix before checkPost wraps it again.
		if !strings.Contains(rec.Body.String(), "Failed to check claim: failed to analyze claim: analyzer unavailable") {
			t.Errorf("expected check-failure error, got body: %s", rec.Body.String())
		}
	})

	t.Run("out of scope renders refusal reason", func(t *testing.T) {
		analyzer := &stubClaimAnalyzer{
			analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
		}
		e, jwtAuth := newCheckTestApp(analyzer, &unreachableClaimJudge{t: t}, &mockrepo.DocumentChunkRepoerMock{}, &mockrepo.AtomicKnowledgeRepoerMock{}, &stubEmbedder{})

		req := formRequest("/check", url.Values{"text": {"what's the weather today?"}})
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not health-related") {
			t.Errorf("expected refusal reason in body, got: %s", rec.Body.String())
		}
	})

	t.Run("in-scope zero-evidence success renders sub-claim verdict", func(t *testing.T) {
		chunkRepo, knowledgeRepo := newEmptySearchMocks()
		analyzer := &stubClaimAnalyzer{
			analysis: &data.ClaimAnalysis{
				InScope:   true,
				SubClaims: []string{"drinking coffee cures cancer"},
				Language:  model.LanguageEnglish,
			},
		}
		e, jwtAuth := newCheckTestApp(analyzer, &unreachableClaimJudge{t: t}, chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{0.1}})

		req := formRequest("/check", url.Values{"text": {"does coffee cure cancer?"}})
		req.AddCookie(mintAuthCookie(t, jwtAuth, &auth.JWTPayload{Role: model.RoleUser}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "drinking coffee cures cancer") {
			t.Errorf("expected sub-claim text in body, got: %s", body)
		}
		if !strings.Contains(body, "insufficient_evidence") {
			t.Errorf("expected insufficient_evidence verdict in body, got: %s", body)
		}
	})
}
