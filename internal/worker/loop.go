package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/storage/postgres"
)

type Processor interface {
	AllowedTypes() []string
	ProcessJob(ctx context.Context, job jobs.Envelope) error
}

func Run(ctx context.Context, logger *slog.Logger, store *postgres.Store, interval time.Duration, workerName string, processor Processor) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		now, err := store.CurrentNow(ctx, time.Now().UTC())
		if err != nil {
			logger.ErrorContext(ctx, "worker_now_failed", "worker", workerName, "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		job, ok, err := store.ClaimJob(ctx, workerName, processor.AllowedTypes(), now)
		if err != nil {
			logger.ErrorContext(ctx, "worker_claim_failed", "worker", workerName, "error", err)
		} else if ok {
			if err := processJob(ctx, logger, store, workerName, processor, job, now.Add(interval)); err != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
