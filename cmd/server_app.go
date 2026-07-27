package cmd

import (
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/core"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/riverqueue/river"
)

// buildAppClaimCheckDeps bundles the claim-checking feature's dependencies
// for buildApp — chunkRepo/knowledgeRepo/embedder are already constructed in
// cmd/server.go for the River workers and are reused here as-is (RAGService
// only calls their read methods), rather than building second instances.
type buildAppClaimCheckDeps struct {
	chunkRepo     data.DocumentChunkRepoer
	knowledgeRepo data.AtomicKnowledgeRepoer
	embedder      data.Embedder
	claimAnalyzer data.ClaimAnalyzer
	claimJudge    data.ClaimJudge
}

// buildApp initializes all API-facing repositories and returns a configured core.App.
// mediaProvider is constructed by the caller (cmd/server.go), shared with any
// River workers that also need it, rather than built again here.
func buildApp(
	cfg *config.Config,
	pool *pgxpool.Pool,
	riverClient *river.Client[pgx.Tx],
	mediaProvider data.MediaUploadProvider,
	claimCheck buildAppClaimCheckDeps,
) *core.App {
	queries := db.New(pool)

	documentRepo := repo.NewDocumentRepo(db.New(pool), pool, riverClient)
	userRepo := repo.NewUserRepo(queries, pool)

	return core.NewApp(
		echo.New(),
		cfg,
		core.WithDocumentRepo(documentRepo),
		core.WithMediaProvider(mediaProvider),
		core.WithChunkRepo(claimCheck.chunkRepo),
		core.WithKnowledgeRepo(claimCheck.knowledgeRepo),
		core.WithEmbedder(claimCheck.embedder),
		core.WithClaimAnalyzer(claimCheck.claimAnalyzer),
		core.WithClaimJudge(claimCheck.claimJudge),
		core.WithUserRepo(userRepo),
	)
}
