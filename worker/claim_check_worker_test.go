package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// stubClaimAnalyzer is a hand-rolled data.ClaimAnalyzer for testing without a
// real analyzer endpoint — same pattern as service package's own test stub.
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

// unreachableClaimJudge fails the test if Judge is ever called — used for
// zero-evidence sub-claims, which must short-circuit before reaching the judge.
type unreachableClaimJudge struct{ t *testing.T }

func (j *unreachableClaimJudge) Judge(context.Context, *data.JudgeRequest) (*data.JudgeVerdict, error) {
	j.t.Fatal("Judge should not be called for a zero-evidence sub-claim")
	return nil, nil
}

// newEmptyRAGService returns a RAGService whose search methods all return no
// results, so any sub-claim short-circuits to InsufficientEvidence.
func newEmptyRAGService() *service.RAGService {
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

	return service.NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{fixed: [][]float32{{0.1}}})
}

func newClaimCheckJob(text string) *river.Job[queue.ClaimCheckJob] {
	return &river.Job[queue.ClaimCheckJob]{
		JobRow: &rivertype.JobRow{ID: 1},
		Args:   queue.ClaimCheckJob{Text: text},
	}
}

func TestClaimCheckWorker_Timeout(t *testing.T) {
	w := NewClaimCheckWorker(nil)
	if got := w.Timeout(newClaimCheckJob("x")); got != claimCheckJobTimeout {
		t.Errorf("Timeout() = %v, want %v", got, claimCheckJobTimeout)
	}
}

func TestClaimCheckWorker_Work_AnalyzerError(t *testing.T) {
	analyzer := &stubClaimAnalyzer{err: errors.New("analyzer unavailable")}
	claimService := service.NewClaimService(analyzer, &unreachableClaimJudge{t: t}, newEmptyRAGService())
	w := NewClaimCheckWorker(claimService)

	err := w.Work(t.Context(), newClaimCheckJob("is coffee healthy?"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestClaimCheckWorker_Work_OutOfScope(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{InScope: false, RefusalReason: "not health-related"},
	}
	claimService := service.NewClaimService(analyzer, &unreachableClaimJudge{t: t}, newEmptyRAGService())
	w := NewClaimCheckWorker(claimService)

	if err := w.Work(t.Context(), newClaimCheckJob("what's the weather today?")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaimCheckWorker_Work_InScopeZeroEvidence(t *testing.T) {
	analyzer := &stubClaimAnalyzer{
		analysis: &data.ClaimAnalysis{
			InScope:   true,
			SubClaims: []string{"drinking coffee cures cancer"},
			Language:  model.LanguageEnglish,
		},
	}
	claimService := service.NewClaimService(analyzer, &unreachableClaimJudge{t: t}, newEmptyRAGService())
	w := NewClaimCheckWorker(claimService)

	if err := w.Work(t.Context(), newClaimCheckJob("does coffee cure cancer?")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
