package cmd

import (
	"os"

	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/external"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/kairosedubf/wobsongo/worker"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP API server",
	Run: func(cmd *cobra.Command, _ []string) {
		cfg := config.NewConfig(EnvFile)
		if err := cfg.IsOK(); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}

		// Initialize database connection pool. Uses repo.NewPgxPool (not
		// pgxpool.New) so pgvector types are registered on every connection —
		// required for document_chunks.embedding to (de)serialize correctly.
		pool, err := repo.NewPgxPool(cmd.Context(), cfg.PostgresURI)
		if err != nil {
			cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
			os.Exit(1)
			return
		}
		defer pool.Close()

		// The media provider is constructed here (not inside buildApp) so it
		// can be shared with River workers, which must be registered before
		// the river.Client (and therefore before buildApp) exists.
		if err := config.IsS3OK(cfg.S3Config); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		mediaProvider, err := repo.NewS3Provider(cmd.Context(), cfg.S3Config)
		if err != nil {
			cmd.PrintErrf("Failed to initialize S3 media provider: %s\n", err.Error())
			os.Exit(1)
			return
		}
		mediaService := service.NewMediaService(mediaProvider)
		doclingClient := external.NewDoclingClient(cfg.DoclingBaseURL)

		if err := config.IsVLMOK(cfg.VLMConfig); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		vlmClient := external.NewVLMClient(
			cfg.VLMConfig.BaseURL,
			cfg.VLMConfig.Model,
			cfg.VLMConfig.APIKey,
		)

		if err := config.IsEmbeddingOK(cfg.EmbeddingConfig); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		embeddingClient := newEmbeddingClient(cfg.EmbeddingConfig)

		if err := config.IsExtractionOK(cfg.ExtractionConfig); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		extractionClient := external.NewExtractionClient(
			cfg.ExtractionConfig.BaseURL,
			cfg.ExtractionConfig.Model,
			cfg.ExtractionConfig.APIKey,
		)

		if err := config.IsTranslationOK(cfg.TranslationConfig); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		translationClient := external.NewTranslationClient(
			cfg.TranslationConfig.BaseURL,
			cfg.TranslationConfig.Model,
			cfg.TranslationConfig.APIKey,
		)

		if err := config.IsClaimCheckOK(cfg.ClaimCheckConfig); err != nil {
			cmd.PrintErrf("Config error: %s\n", err.Error())
			os.Exit(1)
			return
		}
		claimAnalyzerClient := external.NewClaimAnalyzerClient(
			cfg.ClaimCheckConfig.BaseURL,
			cfg.ClaimCheckConfig.Model,
			cfg.ClaimCheckConfig.APIKey,
		)
		judgeClient := external.NewJudgeClient(
			cfg.ClaimCheckConfig.BaseURL,
			cfg.ClaimCheckConfig.Model,
			cfg.ClaimCheckConfig.APIKey,
		)

		// riverClient is assigned below, after workers (which need to be
		// registered via river.AddWorker before river.NewClient produces the
		// client) are constructed. ChunkRepo/RiverJobEnqueuer only resolve it
		// lazily, at Enqueue-call time — always well after riverClient.Start()
		// — so this ordering is safe. See their constructors' doc comments.
		var riverClient *river.Client[pgx.Tx]
		riverClientFn := func() *river.Client[pgx.Tx] { return riverClient }

		chunkRepo := repo.NewDocumentChunkRepo(db.New(pool), pool, riverClientFn)
		jobEnqueuer := repo.NewRiverJobEnqueuer(pool, riverClientFn)

		// Same reasoning as chunkRepo's nil case: this document repo instance
		// is only used by the worker to backfill PageCount/Title after parsing
		// (GetByID+Update) — it never calls Enqueue. The HTTP-facing document
		// repo (with a real riverClient) is built separately, inside buildApp.
		workerDocumentRepo := repo.NewDocumentRepo(db.New(pool), pool, nil)
		documentService := service.NewDocumentService(workerDocumentRepo)

		atomicKnowledgeRepo := repo.NewAtomicKnowledgeRepo(db.New(pool), pool)

		// Register workers with River.
		workers := river.NewWorkers()

		river.AddWorker(workers, worker.NewParseDocumentWorker(
			doclingClient,
			mediaService,
			mediaProvider,
			jobEnqueuer,
		))
		river.AddWorker(workers, worker.NewProcessParsedDocumentWorker(
			mediaProvider,
			documentService,
			chunkRepo,
		))
		river.AddWorker(workers, worker.NewCaptionImageChunksWorker(
			mediaProvider,
			chunkRepo,
			documentService,
			vlmClient,
		))
		river.AddWorker(workers, worker.NewEmbedChunksWorker(chunkRepo, embeddingClient))
		river.AddWorker(workers, worker.NewExtractKnowledgeWorker(
			chunkRepo,
			atomicKnowledgeRepo,
			documentService,
			extractionClient,
			cfg.ExtractionConfig.Concurrency,
		))
		river.AddWorker(workers, worker.NewEmbedKnowledgeWorker(atomicKnowledgeRepo, embeddingClient))
		river.AddWorker(workers, worker.NewTranslateChunksWorker(
			chunkRepo,
			documentService,
			translationClient,
			cfg.TranslationConfig.Concurrency,
		))

		// Initialize River client with the database pool and registered workers.
		// Document ingestion gets its own queue so long-running extractions
		// can't starve other work of worker slots.
		riverClient, err = river.NewClient(riverpgxv5.New(pool), &river.Config{
			Queues: map[string]river.QueueConfig{
				queue.QueueDocumentIngestion: {MaxWorkers: 10},
			},
			Workers: workers,
		})
		if err != nil {
			cmd.PrintErrf("Failed to initialize job queue: %s\n", err.Error())
			os.Exit(1)
			return
		}

		if err := riverClient.Start(cmd.Context()); err != nil {
			cmd.PrintErrf("Failed to start job queue: %s\n", err.Error())
			os.Exit(1)
			return
		}
		defer func() {
			if err := riverClient.Stop(cmd.Context()); err != nil {
				cmd.PrintErrf("Failed to stop River client: %v", err)
				os.Exit(1)
				return
			}
		}()

		// Build and start HTTP API server.
		app := buildApp(cfg, pool, riverClient, mediaProvider, mediaProvider, buildAppClaimCheckDeps{
			chunkRepo:     chunkRepo,
			knowledgeRepo: atomicKnowledgeRepo,
			embedder:      embeddingClient,
			claimAnalyzer: claimAnalyzerClient,
			claimJudge:    judgeClient,
		})

		cmd.Printf("Starting the server at %s\n", cfg.APIHost)
		if err := app.Start(); err != nil {
			cmd.PrintErrf("cannot start the server: %s", err.Error())
			os.Exit(1)
			return
		}
	},
}

// newEmbeddingClient constructs the data.Embedder implementation matching
// cfg.Provider: "openai" for any OpenAI-compatible /v1/embeddings endpoint,
// or "bge" for self-hosted BGE deployments that use a simpler POST shape.
func newEmbeddingClient(cfg *config.EmbeddingConfig) data.Embedder {
	if cfg.Provider == config.EmbeddingProviderBGE {
		return external.NewBGEClient(cfg.BaseURL, cfg.APIKey)
	}
	return external.NewEmbeddingClient(cfg.BaseURL, cfg.Model, cfg.APIKey)
}
