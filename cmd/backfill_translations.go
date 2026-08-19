package cmd

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/db"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/repo"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/spf13/cobra"
)

var backfillTranslationsApply bool

// backfillTranslationsCmd enqueues a TranslateChunksDTO job for every
// document that has at least one chunk with text but no translation yet —
// needed for documents ingested before TranslateChunksDTO was enqueued
// automatically at ingestion time (see ProcessParsedDocumentWorker/
// CaptionImageChunksWorker). Actual translation still happens via the
// normal TranslateChunksWorker, run by wob serve; this command only enqueues.
var backfillTranslationsCmd = &cobra.Command{
	Use:   "backfill-translations",
	Short: "Enqueue translation for every document with untranslated chunks",
	Long: "Finds every document with at least one chunk that has text but no\n" +
		"translation yet, and enqueues a TranslateChunksDTO job for it. Translation\n" +
		"itself is performed by TranslateChunksWorker, which only runs inside\n" +
		"`wob serve` — this command just fans the backlog out onto the queue.\n\n" +
		"Without --apply, this is a dry run: it reports how many documents would\n" +
		"be enqueued, but enqueues nothing.",
	Run: runBackfillTranslations,
}

func init() {
	backfillTranslationsCmd.Flags().
		BoolVar(&backfillTranslationsApply, "apply", false, "Actually enqueue the jobs (default is a dry run)")
}

// registrationOnlyTranslateChunksWorker satisfies River's client-side check
// that a job's kind is registered before Insert/InsertTx is allowed to
// enqueue it. This CLI process never calls riverClient.Start(), so Work is
// never actually invoked — it exists purely to make the registration check pass.
type registrationOnlyTranslateChunksWorker struct {
	river.WorkerDefaults[queue.TranslateChunksDTO]
}

func (*registrationOnlyTranslateChunksWorker) Work(
	context.Context,
	*river.Job[queue.TranslateChunksDTO],
) error {
	return nil
}

func runBackfillTranslations(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()
	cfg := appconfig.NewConfig(EnvFile)

	pool, err := pgxpool.New(ctx, cfg.PostgresURI)
	if err != nil {
		cmd.PrintErrf("Failed to connect to database: %s\n", err.Error())
		os.Exit(1)
		return
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &registrationOnlyTranslateChunksWorker{})
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	if err != nil {
		cmd.PrintErrf("Failed to initialize job queue client: %s\n", err.Error())
		os.Exit(1) //nolint:gocritic // process exit; same accepted pattern as cmd/server.go
		return
	}

	chunkRepo := repo.NewDocumentChunkRepo(
		db.New(pool),
		pool,
		func() *river.Client[pgx.Tx] { return riverClient },
	)

	documentIDs, err := chunkRepo.ListDocumentIDsNeedingTranslation(ctx)
	if err != nil {
		cmd.PrintErrf("Failed to list documents needing translation: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if len(documentIDs) == 0 {
		cmd.Println("No documents have untranslated chunks.")
		return
	}

	if !backfillTranslationsApply {
		cmd.Printf("Dry run: would enqueue translation for %d document(s):\n", len(documentIDs))
		for _, id := range documentIDs {
			cmd.Printf("  %s\n", id)
		}
		cmd.Println("Re-run with --apply to actually enqueue these jobs.")
		return
	}

	for _, id := range documentIDs {
		if err := chunkRepo.Enqueue(ctx, queue.TranslateChunksDTO{DocumentID: id}); err != nil {
			cmd.PrintErrf("Failed to enqueue translation for document %s: %s\n", id, err.Error())
			os.Exit(1)
			return
		}
	}
	cmd.Printf("Enqueued translation for %d document(s).\n", len(documentIDs))
}
