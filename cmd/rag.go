package cmd

import (
	"fmt"
	"os"
	"strings"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/spf13/cobra"
)

// ragDefaultLimit bounds how many fused results are printed. Not worth a
// flag yet — this is a test harness for the retrieval layer, not a tuned
// production query surface.
const ragDefaultLimit = 10

var ragCmd = &cobra.Command{
	Use:   "rag [query]",
	Short: "Run hybrid search (vector + full-text + trigram) across chunks and atomic-knowledge facts",
	Long: "Embeds the query once, then searches five ways concurrently — chunk\n" +
		"vector similarity, chunk full-text search, fact vector similarity, fact\n" +
		"full-text search, and fact trigram fuzzy match against subject/\n" +
		"predicate/object — fusing all five ranked lists via Reciprocal Rank\n" +
		"Fusion. Read-only: no --apply flag, nothing is mutated.",
	Args: cobra.ExactArgs(1),
	Run:  runRAG,
}

func runRAG(cmd *cobra.Command, args []string) {
	query := args[0]
	cfg := appconfig.NewConfig(EnvFile)

	if err := appconfig.IsEmbeddingOK(cfg.EmbeddingConfig); err != nil {
		cmd.PrintErrf("Config error: %s\n", err.Error())
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
	// justification as workerDocumentRepo in cmd/server.go.
	chunkRepo := repo.NewDocumentChunkRepo(db.New(pool), pool, nil)
	knowledgeRepo := repo.NewAtomicKnowledgeRepo(db.New(pool), pool)
	embeddingClient := newEmbeddingClient(cfg.EmbeddingConfig)

	ragService := service.NewRAGService(chunkRepo, knowledgeRepo, embeddingClient)
	results, err := ragService.Search(ctx, query, ragDefaultLimit)
	if err != nil {
		cmd.PrintErrf("Search failed: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if len(results) == 0 {
		cmd.Println("No results.")
		return
	}

	for i, r := range results {
		cmd.Printf(
			"%d. [%s] score=%.4f methods=%s doc=%s lang=%s\n",
			i+1, r.Source, r.RRFScore, strings.Join(r.Methods, ","), r.DocumentID, r.Language,
		)
		if r.Source == "fact" {
			cmd.Printf("    truth_tier=%s\n", r.TruthTier)
		} else if r.Page > 0 {
			cmd.Printf("    page=%d\n", r.Page)
		}
		cmd.Printf("    %s\n", truncateForDisplay(r.Text, 300))
	}
}

// truncateForDisplay caps s to at most n runes for terminal-friendly output,
// appending an ellipsis if it was cut.
func truncateForDisplay(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return fmt.Sprintf("%s...", string(runes[:n]))
}
