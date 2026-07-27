package cmd

import (
	"encoding/json"
	"os"
	"slices"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/spf13/cobra"
)

// bilingualEvalFixture is a single golden-set case: the same underlying
// question asked in both supported languages, which must surface the same
// evidence regardless of which one the user actually typed.
type bilingualEvalFixture struct {
	Label         string `json:"label"`
	EnglishQuery  string `json:"english_query"`
	FrenchQuery   string `json:"french_query"`
	ExpectedPages []int  `json:"expected_pages"`
}

// bilingualEvalTopN bounds how many fused results are considered when
// checking whether an expected page was actually surfaced — matches
// ragDefaultLimit, since that's the same top slice a user would actually see.
const bilingualEvalTopN = ragDefaultLimit

var evalRetrievalBilingualCmd = &cobra.Command{
	Use:   "eval-retrieval-bilingual [fixture-file]",
	Short: "Check that French and English queries surface the same evidence from the KB",
	Long: "Runs both language versions of the same underlying question through the real\n" +
		"hybrid-search retrieval layer and checks that each of the fixture's expected pages\n" +
		"is actually surfaced (via a real full-text match, not vector-only) for BOTH\n" +
		"queries. This is the regression test for the cross-lingual ranking gap this\n" +
		"system was built to fix — it costs a real embedding call per query and isn't\n" +
		"wired into go test/CI. Read-only: no --apply flag, nothing is mutated.",
	Args: cobra.ExactArgs(1),
	Run:  runEvalRetrievalBilingual,
}

func runEvalRetrievalBilingual(cmd *cobra.Command, args []string) {
	fixturePath := args[0]
	cfg := appconfig.NewConfig(EnvFile)

	if err := appconfig.IsEmbeddingOK(cfg.EmbeddingConfig); err != nil {
		cmd.PrintErrf("Config error: %s\n", err.Error())
		os.Exit(1)
		return
	}

	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		cmd.PrintErrf("Failed to read fixture file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	var fixtures []bilingualEvalFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		cmd.PrintErrf("Failed to parse fixture file: %s\n", err.Error())
		os.Exit(1)
		return
	}

	ctx := cmd.Context()
	pool, err := repo.NewPgxPool(ctx, cfg.PostgresURI)
	if err != nil {
		cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
		os.Exit(1)
		return
	}
	defer pool.Close()

	// nil riverClient: this command only reads, never enqueues — same
	// justification as cmd/rag.go.
	chunkRepo := repo.NewDocumentChunkRepo(db.New(pool), pool, nil)
	knowledgeRepo := repo.NewAtomicKnowledgeRepo(db.New(pool), pool)
	embeddingClient := newEmbeddingClient(cfg.EmbeddingConfig)
	ragService := service.NewRAGService(chunkRepo, knowledgeRepo, embeddingClient)

	var passed, failed int
	for _, fixture := range fixtures {
		enHits, err := ragService.Search(ctx, fixture.EnglishQuery, bilingualEvalTopN)
		if err != nil {
			failed++
			cmd.Printf("FAIL %s: English search failed: %s\n", fixture.Label, err.Error())
			continue
		}
		frHits, err := ragService.Search(ctx, fixture.FrenchQuery, bilingualEvalTopN)
		if err != nil {
			failed++
			cmd.Printf("FAIL %s: French search failed: %s\n", fixture.Label, err.Error())
			continue
		}

		var missing []int
		for _, page := range fixture.ExpectedPages {
			if !pageFoundViaFullText(enHits, page) {
				missing = append(missing, page)
			}
			if !pageFoundViaFullText(frHits, page) {
				missing = append(missing, page)
			}
		}

		if len(missing) == 0 {
			passed++
			cmd.Printf("PASS %s\n", fixture.Label)
			continue
		}

		failed++
		cmd.Printf(
			"FAIL %s: expected pages %v via full-text match in both languages' top %d, missing/vector-only: %v\n",
			fixture.Label,
			fixture.ExpectedPages,
			bilingualEvalTopN,
			missing,
		)
	}

	cmd.Printf("\n%d/%d passed\n", passed, len(fixtures))
	if failed > 0 {
		os.Exit(1)
	}
}

// pageFoundViaFullText reports whether hits contains a chunk hit for page
// whose Methods includes "fts" — a vector-only hit doesn't count, since the
// whole point of this eval is confirming the full-text index itself now
// matches cross-lingually, not just that embeddings are similar enough.
func pageFoundViaFullText(hits []service.RAGResult, page int) bool {
	for _, h := range hits {
		if h.Source == "chunk" && h.Page == page && slices.Contains(h.Methods, "fts") {
			return true
		}
	}
	return false
}
