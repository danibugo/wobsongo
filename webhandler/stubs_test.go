package webhandler_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
)

// stubMediaProvider is a hand-rolled data.MediaUploadProvider for testing
// without real S3/MinIO — mirrors worker/parse_document_test.go's stub.
type stubMediaProvider struct {
	presignedURL string
	err          error
}

func (s *stubMediaProvider) GetPresignedPOSTURL(
	context.Context, data.MediaUploadIntent, string, string,
) (*url.URL, map[string]string, error) {
	panic("not used in this test")
}

func (s *stubMediaProvider) GetPresignedGETURL(context.Context, string, int64) (string, error) {
	return s.presignedURL, s.err
}

func (s *stubMediaProvider) GetPresignedGETURLs(
	context.Context, []string, int64,
) (map[string]string, error) {
	panic("not used in this test")
}

// panicIfGETCalledMediaProvider panics if GetPresignedGETURL is ever called —
// used to prove a non-PDF-document code path never reaches the media
// provider at all.
type panicIfGETCalledMediaProvider struct{}

func (panicIfGETCalledMediaProvider) GetPresignedPOSTURL(
	context.Context, data.MediaUploadIntent, string, string,
) (*url.URL, map[string]string, error) {
	panic("not used in this test")
}

func (panicIfGETCalledMediaProvider) GetPresignedGETURL(context.Context, string, int64) (string, error) {
	panic("GetPresignedGETURL should not be called for a non-PDF document")
}

func (panicIfGETCalledMediaProvider) GetPresignedGETURLs(
	context.Context, []string, int64,
) (map[string]string, error) {
	panic("not used in this test")
}

// stubEmbedder is a hand-rolled data.Embedder for testing without a real
// embeddings provider — mirrors worker/embed_chunks_test.go's stub.
type stubEmbedder struct {
	vector []float32
	err    error
}

func (s *stubEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return [][]float32{s.vector}, nil
}

// stubClaimAnalyzer is a hand-rolled data.ClaimAnalyzer for testing without a
// real analyzer endpoint — mirrors worker/claim_check_worker_test.go's stub.
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

// newEmptySearchMocks returns a DocumentChunkRepoerMock and
// AtomicKnowledgeRepoerMock whose search methods all return no results, for
// tests where the RAGService (built internally by RegisterWebRoutes from
// WebRepos) should find nothing.
func newEmptySearchMocks() (*mockrepo.DocumentChunkRepoerMock, *mockrepo.AtomicKnowledgeRepoerMock) {
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

	return chunkRepo, knowledgeRepo
}
