package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
)

// stubClaimAnalyzer is a hand-rolled data.ClaimAnalyzer for testing without a
// real analyzer endpoint — same pattern as stubEmbedder in rag_test.go.
type stubClaimAnalyzer struct {
	analysis *data.ClaimAnalysis
	err      error
}

func (s *stubClaimAnalyzer) Analyze(context.Context, string) (*data.ClaimAnalysis, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.analysis, nil
}

// stubClaimJudge is a hand-rolled data.ClaimJudge for testing. calls records
// every claim it was actually invoked with (mutex-protected since
// ClaimService judges sub-claims concurrently) — used to assert a sub-claim
// with no retrieved evidence never reaches the judge at all.
type stubClaimJudge struct {
	mu        sync.Mutex
	calls     []string
	judgeFunc func(req *data.JudgeRequest) (*data.JudgeVerdict, error)
}

func (s *stubClaimJudge) Judge(
	_ context.Context,
	req *data.JudgeRequest,
) (*data.JudgeVerdict, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req.Claim)
	s.mu.Unlock()
	return s.judgeFunc(req)
}

func (s *stubClaimJudge) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// newEmptyRAGService returns a RAGService whose five search methods all
// return no results, for tests that don't care about retrieval content.
func newEmptyRAGService() *RAGService {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}

	return NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
}

func TestClaimService_CheckClaim_OutOfScopeShortCircuitsBeforeRetrieval(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called for out-of-scope input")
			return nil, nil
		},
	}

	s := NewClaimService(analyzer, judge, newEmptyRAGService())
	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "what's the capital of France?"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InScope {
		t.Error("expected InScope false")
	}
	if result.RefusalReason != "not health-related" {
		t.Errorf("expected refusal reason to propagate, got %q", result.RefusalReason)
	}
	if len(result.SubClaims) != 0 {
		t.Errorf("expected no sub-claims for out-of-scope input, got %d", len(result.SubClaims))
	}
	if judge.callCount() != 0 {
		t.Errorf("expected judge never called, got %d calls", judge.callCount())
	}
}

func TestClaimService_CheckClaim_ZeroEvidenceNeverReachesJudge(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []string{"an obscure unverifiable claim"},
		},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(*data.JudgeRequest) (*data.JudgeVerdict, error) {
			t.Fatal("judge should never be called when retrieval returns no evidence")
			return nil, nil
		},
	}

	s := NewClaimService(analyzer, judge, newEmptyRAGService())
	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "an obscure unverifiable claim"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.InScope {
		t.Fatal("expected InScope true")
	}
	if len(result.SubClaims) != 1 {
		t.Fatalf("expected 1 sub-claim result, got %d", len(result.SubClaims))
	}
	if result.SubClaims[0].Verdict != model.VerdictInsufficientEvidence {
		t.Errorf("expected InsufficientEvidence, got %s", result.SubClaims[0].Verdict)
	}
	if judge.callCount() != 0 {
		t.Errorf("expected judge never called, got %d calls", judge.callCount())
	}
}

func TestClaimService_CheckClaim_MultipleSubClaimsJudgedConcurrentlyAndAggregated(t *testing.T) {
	docID := uuid.New()
	factChunkID := uuid.New()

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		_ context.Context, query string, _ int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		if query == "claim A" {
			return []data.ScoredResult[model.DocumentChunk]{
				{
					Item: model.DocumentChunk{
						ID:          uuid.New(),
						DocumentID:  docID,
						ParsedChunk: model.ParsedChunk{Text: "supporting text"},
					},
					Score: 0.9,
				},
			}, nil
		}
		return nil, nil
	}
	// hydrateFactChunks fetches the "claim B" fact's parent chunk by ID.
	chunkRepo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
		if id != factChunkID {
			t.Fatalf("GetByID called with unexpected id %s", id)
		}
		return &model.DocumentChunk{
			ID:          factChunkID,
			DocumentID:  docID,
			ParsedChunk: model.ParsedChunk{Text: "contradicting passage"},
		}, nil
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}
	knowledgeRepo.SearchByFullTextFunc = func(
		_ context.Context, query string, _ int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		if query == "claim B" {
			return []data.ScoredResult[model.AtomicKnowledge]{
				{
					Item: model.AtomicKnowledge{
						ID:              uuid.New(),
						DocumentID:      docID,
						DocumentChunkID: factChunkID,
						TruthTier:       model.TruthTierAxiomatic,
					},
					Score: 0.9,
				},
			}, nil
		}
		return nil, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return nil, nil
	}

	rag := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})

	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: true, SubClaims: []string{"claim A", "claim B"}},
	}
	judge := &stubClaimJudge{
		judgeFunc: func(req *data.JudgeRequest) (*data.JudgeVerdict, error) {
			switch req.Claim {
			case "claim A":
				return &data.JudgeVerdict{
					Verdict:       model.VerdictSupported,
					CitedEvidence: []int{0},
				}, nil
			case "claim B":
				return &data.JudgeVerdict{
					Verdict:       model.VerdictContradicted,
					CitedEvidence: []int{0},
				}, nil
			default:
				t.Fatalf("unexpected claim %q", req.Claim)
				return nil, nil
			}
		},
	}

	s := NewClaimService(analyzer, judge, rag)
	result, err := s.CheckClaim(t.Context(), &dto.CheckClaimDTO{Text: "claim A and claim B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.InScope {
		t.Fatal("expected InScope true")
	}
	if len(result.SubClaims) != 2 {
		t.Fatalf("expected 2 sub-claim results, got %d", len(result.SubClaims))
	}
	// Order must match the analyzer's SubClaims order regardless of which
	// goroutine finished first.
	if result.SubClaims[0].Claim != "claim A" ||
		result.SubClaims[0].Verdict != model.VerdictSupported {
		t.Errorf("expected sub-claim 0 = claim A/Supported, got %+v", result.SubClaims[0])
	}
	if result.SubClaims[1].Claim != "claim B" ||
		result.SubClaims[1].Verdict != model.VerdictContradicted {
		t.Errorf("expected sub-claim 1 = claim B/Contradicted, got %+v", result.SubClaims[1])
	}
	if result.OverallSummary != "contains inaccuracies" {
		t.Errorf(
			"expected overall summary to reflect the contradiction, got %q",
			result.OverallSummary,
		)
	}

	wantMessage := "❌ contains inaccuracies — 2 claims checked\n\n" +
		"✅ 1. claim A\n\n" +
		"❌ 2. claim B"
	if result.FormattedMessage != wantMessage {
		t.Errorf(
			"expected formatted message %q, got %q",
			wantMessage, result.FormattedMessage,
		)
	}
}

func TestClaimService_CheckClaim_OutOfScopeHasNoFormattedMessage(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	s := NewClaimService(analyzer, &stubClaimJudge{}, newEmptyRAGService())

	result, err := s.CheckClaim(
		t.Context(),
		&dto.CheckClaimDTO{Text: "what's the capital of France?"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FormattedMessage != "" {
		t.Errorf(
			"expected no formatted message for out-of-scope input, got %q",
			result.FormattedMessage,
		)
	}
}
