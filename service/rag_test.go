package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/mockrepo"
	"github.com/kairosedubf/wobsongo/model"
)

// stubEmbedder is a hand-rolled data.Embedder for testing without a real
// embeddings provider.
type stubEmbedder struct {
	vector []float32
	err    error
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	vectors := make([][]float32, len(texts))
	for i := range vectors {
		vectors[i] = s.vector
	}
	return vectors, nil
}

func TestFuseRRF_ItemInMultipleListsOutranksSingleList(t *testing.T) {
	inBoth := RAGResult{Key: "chunk:a", Source: "chunk", Methods: []string{"vector"}}
	inOne := RAGResult{Key: "chunk:b", Source: "chunk", Methods: []string{"vector"}}

	// inBoth appears first in both lists (best rank in each); inOne appears
	// first in only one list. inBoth must outrank inOne despite identical
	// per-list rank, since it accumulates RRF contributions from two lists.
	listA := []RAGResult{inBoth, inOne}
	listB := []RAGResult{{Key: "chunk:a", Source: "chunk", Methods: []string{"fts"}}}

	fused := fuseRRF(listA, listB)

	if len(fused) != 2 {
		t.Fatalf("expected 2 fused results, got %d", len(fused))
	}
	if fused[0].Key != "chunk:a" {
		t.Errorf("expected chunk:a (hit by both lists) ranked first, got %s", fused[0].Key)
	}
	if fused[0].RRFScore <= fused[1].RRFScore {
		t.Errorf(
			"expected chunk:a's score (%v) to exceed chunk:b's (%v)",
			fused[0].RRFScore, fused[1].RRFScore,
		)
	}
}

func TestFuseRRF_DedupesByKeyAndAccumulatesMethods(t *testing.T) {
	listA := []RAGResult{{Key: "fact:x", Source: "fact", Methods: []string{"vector"}}}
	listB := []RAGResult{{Key: "fact:x", Source: "fact", Methods: []string{"fts"}}}
	listC := []RAGResult{{Key: "fact:x", Source: "fact", Methods: []string{"trgm"}}}

	fused := fuseRRF(listA, listB, listC)

	if len(fused) != 1 {
		t.Fatalf("expected exactly 1 fused result (deduped by Key), got %d", len(fused))
	}
	if len(fused[0].Methods) != 3 {
		t.Fatalf("expected all 3 methods accumulated, got %v", fused[0].Methods)
	}
	want := map[string]bool{"vector": true, "fts": true, "trgm": true}
	for _, m := range fused[0].Methods {
		if !want[m] {
			t.Errorf("unexpected method %q in %v", m, fused[0].Methods)
		}
	}
}

func TestFuseRRF_OrdersByCombinedScoreDescending(t *testing.T) {
	// Three single-list items at different ranks — score must strictly
	// decrease as rank worsens.
	list := []RAGResult{
		{Key: "chunk:first", Methods: []string{"vector"}},
		{Key: "chunk:second", Methods: []string{"vector"}},
		{Key: "chunk:third", Methods: []string{"vector"}},
	}

	fused := fuseRRF(list)

	if len(fused) != 3 {
		t.Fatalf("expected 3 results, got %d", len(fused))
	}
	for i := range fused[:len(fused)-1] {
		if fused[i].RRFScore <= fused[i+1].RRFScore {
			t.Errorf(
				"expected strictly descending scores, got %v then %v at index %d",
				fused[i].RRFScore, fused[i+1].RRFScore, i,
			)
		}
	}
}

func TestRAGService_Search_FusesAcrossAllFiveMethods(t *testing.T) {
	chunkID := uuid.New()
	factID := uuid.New()
	docID := uuid.New()

	chunk := model.DocumentChunk{
		ID:         chunkID,
		DocumentID: docID,
		ParsedChunk: model.ParsedChunk{
			Text: "chunk text",
			Page: 3,
		},
	}
	fact := model.AtomicKnowledge{
		ID:              factID,
		DocumentID:      docID,
		DocumentChunkID: chunkID,
		TruthTier:       model.TruthTierAxiomatic,
		Subject:         "Alice",
		Predicate:       "founded",
		Object:          "Acme",
	}

	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return []data.ScoredResult[model.DocumentChunk]{{Item: chunk, Score: 0.1}}, nil
	}
	chunkRepo.SearchByFullTextFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, nil
	}
	// hydrateFactChunks fetches the fact's parent chunk by ID — same chunkID
	// as the chunk hit above, so this also exercises a fact and its own
	// source chunk both appearing in one fused result set.
	chunkRepo.GetByIDFunc = func(_ context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
		if id != chunkID {
			t.Fatalf("GetByID called with unexpected id %s", id)
		}
		return &chunk, nil
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
		return []data.ScoredResult[model.AtomicKnowledge]{{Item: fact, Score: 0.9}}, nil
	}
	knowledgeRepo.SearchBySimilarityFunc = func(
		context.Context, string, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return []data.ScoredResult[model.AtomicKnowledge]{{Item: fact, Score: 0.5}}, nil
	}

	s := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1, 2, 3}})
	results, err := s.Search(t.Context(), "some query", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 fused results (1 chunk, 1 fact), got %d: %+v", len(results), results)
	}

	// The fact was hit by both SearchByFullText and SearchBySimilarity, the
	// chunk only by SearchByEmbedding — the fact must rank first.
	if results[0].Key != "fact:"+factID.String() {
		t.Errorf("expected the fact (hit by 2 methods) ranked first, got %s", results[0].Key)
	}
	if len(results[0].Methods) != 2 {
		t.Errorf("expected the fact to accumulate 2 methods, got %v", results[0].Methods)
	}
	if results[0].TruthTier != "axiomatic" {
		t.Errorf("expected TruthTier %q, got %q", "axiomatic", results[0].TruthTier)
	}
	if results[0].ChunkText != "chunk text" {
		t.Errorf(
			"expected the fact hit's ChunkText hydrated from its parent chunk, got %q",
			results[0].ChunkText,
		)
	}
	if results[1].Key != "chunk:"+chunkID.String() {
		t.Errorf("expected the chunk ranked second, got %s", results[1].Key)
	}
	if results[1].Page != 3 {
		t.Errorf("expected chunk Page 3, got %d", results[1].Page)
	}
	if results[1].ChunkText != "" {
		t.Errorf(
			"expected a chunk-source hit's ChunkText to stay empty, got %q",
			results[1].ChunkText,
		)
	}
}

func TestRAGService_Search_LimitsFinalResultCount(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return []data.ScoredResult[model.DocumentChunk]{
			{Item: model.DocumentChunk{ID: uuid.New()}, Score: 0.1},
			{Item: model.DocumentChunk{ID: uuid.New()}, Score: 0.2},
			{Item: model.DocumentChunk{ID: uuid.New()}, Score: 0.3},
		}, nil
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

	s := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
	results, err := s.Search(t.Context(), "query", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected results truncated to limit 2, got %d", len(results))
	}
}

func TestRAGService_Search_EmbedderError(t *testing.T) {
	s := NewRAGService(
		&mockrepo.DocumentChunkRepoerMock{},
		&mockrepo.AtomicKnowledgeRepoerMock{},
		&stubEmbedder{err: errors.New("embedding endpoint down")},
	)
	if _, err := s.Search(t.Context(), "query", 10); err == nil {
		t.Fatal("expected an error when the embedder fails")
	}
}

func TestRAGService_Search_RepoErrorPropagates(t *testing.T) {
	chunkRepo := &mockrepo.DocumentChunkRepoerMock{}
	chunkRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.DocumentChunk], error) {
		return nil, errors.New("db down")
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

	s := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
	if _, err := s.Search(t.Context(), "query", 10); err == nil {
		t.Fatal("expected an error when a repo search fails")
	}
}

func TestRAGService_Search_HydrationErrorPropagates(t *testing.T) {
	fact := model.AtomicKnowledge{
		ID:              uuid.New(),
		DocumentChunkID: uuid.New(),
		TruthTier:       model.TruthTierAxiomatic,
	}

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
	chunkRepo.GetByIDFunc = func(context.Context, uuid.UUID) (*model.DocumentChunk, error) {
		return nil, errors.New("chunk not found")
	}

	knowledgeRepo := &mockrepo.AtomicKnowledgeRepoerMock{}
	knowledgeRepo.SearchByEmbeddingFunc = func(
		context.Context, []float32, int,
	) ([]data.ScoredResult[model.AtomicKnowledge], error) {
		return []data.ScoredResult[model.AtomicKnowledge]{{Item: fact, Score: 0.9}}, nil
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

	s := NewRAGService(chunkRepo, knowledgeRepo, &stubEmbedder{vector: []float32{1}})
	if _, err := s.Search(t.Context(), "query", 10); err == nil {
		t.Fatal("expected an error when hydrating a fact's parent chunk fails")
	}
}
