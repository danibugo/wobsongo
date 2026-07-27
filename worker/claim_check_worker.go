package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kairosedubf/wobsongo/dto"
	"github.com/kairosedubf/wobsongo/queue"
	"github.com/kairosedubf/wobsongo/service"
	"github.com/riverqueue/river"
)

const claimCheckJobTimeout = 3 * time.Minute

// ClaimCheckWorker runs the full claim-check pipeline (analyze → retrieve →
// judge) for a piece of text. For async callback support (e.g. WhatsApp bot
// notifications), use a commercial extension that wraps ClaimService directly.
type ClaimCheckWorker struct {
	river.WorkerDefaults[queue.ClaimCheckJob]
	claimService *service.ClaimService
}

// NewClaimCheckWorker creates a new ClaimCheckWorker.
func NewClaimCheckWorker(claimService *service.ClaimService) *ClaimCheckWorker {
	return &ClaimCheckWorker{claimService: claimService}
}

func (w *ClaimCheckWorker) Timeout(_ *river.Job[queue.ClaimCheckJob]) time.Duration {
	return claimCheckJobTimeout
}

// Work runs the claim check pipeline and logs the result.
func (w *ClaimCheckWorker) Work(ctx context.Context, job *river.Job[queue.ClaimCheckJob]) error {
	log.Printf("[ClaimCheckWorker] Starting claim check for job %d", job.ID)

	result, err := w.claimService.CheckClaim(ctx, &dto.CheckClaimDTO{Text: job.Args.Text})
	if err != nil {
		return fmt.Errorf("claim check failed: %w", err)
	}

	message := result.FormattedMessage
	if !result.InScope {
		message = result.RefusalReason
	}

	log.Printf("[ClaimCheckWorker] Completed job %d: %s", job.ID, message)
	return nil
}
