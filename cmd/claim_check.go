package cmd

import (
	"os"

	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/spf13/cobra"
)

var claimCheckCmd = &cobra.Command{
	Use:   "claim-check [claim]",
	Short: "Check a claim against the knowledge base and print the reply as the WhatsApp bot would show it",
	Long: "Runs the full claim-checking pipeline — scope/decomposition, per-sub-claim\n" +
		"hybrid-search retrieval (reusing the same retrieval layer as `wob rag`), and\n" +
		"verdict judging — against the real dev DB and configured LLM endpoints, then\n" +
		"prints exactly the text a chat client (e.g. the WhatsApp bot) would display —\n" +
		"simulating that reply without needing the bot's WhatsApp connection.\n" +
		"Read-only: no --apply flag, nothing is mutated.",
	Args: cobra.ExactArgs(1),
	Run:  runClaimCheck,
}

func runClaimCheck(cmd *cobra.Command, args []string) {
	claim := args[0]
	cfg := appconfig.NewConfig(EnvFile)

	if err := appconfig.IsEmbeddingOK(cfg.EmbeddingConfig); err != nil {
		cmd.PrintErrf("Config error: %s\n", err.Error())
		os.Exit(1)
		return
	}
	if err := appconfig.IsClaimCheckOK(cfg.ClaimCheckConfig); err != nil {
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
	// justification as cmd/rag.go's chunkRepo.
	chunkRepo := repo.NewDocumentChunkRepo(db.New(pool), pool, nil)
	knowledgeRepo := repo.NewAtomicKnowledgeRepo(db.New(pool), pool)
	embeddingClient := newEmbeddingClient(cfg.EmbeddingConfig)

	analyzerClient := external.NewClaimAnalyzerClient(
		cfg.ClaimCheckConfig.BaseURL,
		cfg.ClaimCheckConfig.Model,
		cfg.ClaimCheckConfig.APIKey,
	)
	judgeClient := external.NewJudgeClient(
		cfg.ClaimCheckConfig.BaseURL,
		cfg.ClaimCheckConfig.Model,
		cfg.ClaimCheckConfig.APIKey,
	)

	ragService := service.NewRAGService(chunkRepo, knowledgeRepo, embeddingClient)
	claimService := service.NewClaimService(analyzerClient, judgeClient, ragService)

	result, err := claimService.CheckClaim(ctx, &dto.CheckClaimDTO{Text: claim})
	if err != nil {
		cmd.PrintErrf("Claim check failed: %s\n", err.Error())
		os.Exit(1) //nolint:gocritic // process exit; same accepted pattern as cmd/server.go
		return
	}

	printClaimCheckResult(cmd, result)
}

// printClaimCheckResult prints exactly the text a chat client would display
// to the user — result.RefusalReason when out of scope (the only user-facing
// text that path produces), otherwise result.FormattedMessage, the same
// color-coded, emoji-per-verdict rendering the API returns for a bot to show
// as-is (see internal/service/claim_service.go's formatClaimMessage).
func printClaimCheckResult(cmd *cobra.Command, result *service.ClaimCheckResult) {
	if !result.InScope {
		cmd.Println(result.RefusalReason)
		return
	}
	cmd.Println(result.FormattedMessage)
}
