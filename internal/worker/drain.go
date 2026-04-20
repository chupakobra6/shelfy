package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
)

func DrainDue(ctx context.Context, logger *slog.Logger, store *postgres.Store, workerName string, processor Processor, limit int) (processed int, limitReached bool, err error) {
	if limit <= 0 {
		limit = 100
	}
	for processed < limit {
		now, err := store.CurrentNow(ctx, time.Now().UTC())
		if err != nil {
			return processed, false, err
		}
		job, ok, err := store.ClaimJob(ctx, workerName, processor.AllowedTypes(), now)
		if err != nil {
			return processed, false, err
		}
		if !ok {
			return processed, false, nil
		}
		if err := processJob(ctx, logger, store, workerName, processor, job, now.Add(time.Second)); err != nil {
			return processed, false, err
		}
		processed++
	}
	return processed, true, nil
}

func processJob(ctx context.Context, logger *slog.Logger, store *postgres.Store, workerName string, processor Processor, job jobs.Envelope, retryAt time.Time) error {
	jobCtx := observability.WithTraceID(ctx, job.TraceID)
	jobCtx = observability.WithJobID(jobCtx, job.ID)
	if err := processor.ProcessJob(jobCtx, job); err != nil {
		logger.ErrorContext(jobCtx, "worker_job_failed", observability.LogAttrs(jobCtx,
			"worker", workerName,
			"job_type", job.JobType,
			"error", err,
		)...)
		if retryErr := store.MarkJobRetry(jobCtx, job.ID, retryAt, err.Error()); retryErr != nil {
			logger.ErrorContext(jobCtx, "worker_mark_retry_failed", observability.LogAttrs(jobCtx, "error", retryErr)...)
		}
		return err
	}
	if doneErr := store.MarkJobDone(jobCtx, job.ID); doneErr != nil {
		logger.ErrorContext(jobCtx, "worker_mark_done_failed", observability.LogAttrs(jobCtx, "error", doneErr)...)
		return doneErr
	}
	logger.InfoContext(jobCtx, "worker_job_done", observability.LogAttrs(jobCtx, "worker", workerName, "job_type", job.JobType)...)
	return nil
}
