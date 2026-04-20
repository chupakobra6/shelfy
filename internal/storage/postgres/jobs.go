package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

func (s *Store) EnqueueJob(ctx context.Context, traceID, jobType string, payload any, runAt time.Time, idempotencyKey *string) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rowsAffected, err := s.queries.EnqueueJob(ctx, sqlcgen.EnqueueJobParams{
		TraceID:        traceID,
		JobType:        jobType,
		IdempotencyKey: idempotencyKey,
		Payload:        encoded,
		RunAt:          pgTimestamptzFromTime(runAt),
	})
	if err == nil && rowsAffected == 1 {
		s.logJobEvent(ctx, slog.LevelInfo, slog.LevelDebug, jobType, "job_enqueued", observability.LogAttrs(ctx,
			"job_type", jobType,
			"run_at", runAt.Format(time.RFC3339),
			"idempotency_key", idempotencyKey,
		)...)
	} else if err == nil {
		s.logJobEvent(ctx, slog.LevelInfo, slog.LevelDebug, jobType, "job_enqueue_skipped", observability.LogAttrs(ctx,
			"job_type", jobType,
			"run_at", runAt.Format(time.RFC3339),
			"idempotency_key", idempotencyKey,
		)...)
	}
	return err
}

func (s *Store) ClaimJob(ctx context.Context, workerName string, allowedTypes []string, now time.Time) (jobs.Envelope, bool, error) {
	row, err := s.queries.ClaimJob(ctx, sqlcgen.ClaimJobParams{
		RunAt:   pgTimestamptzFromTime(now),
		Column2: allowedTypes,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Envelope{}, false, nil
	}
	if err != nil {
		return jobs.Envelope{}, false, err
	}
	job := jobs.Envelope{
		ID:             row.ID,
		TraceID:        row.TraceID,
		JobType:        row.JobType,
		Status:         domain.JobStatus(row.Status),
		Payload:        row.Payload,
		RunAt:          timeFromPgTimestamptz(row.RunAt),
		Attempts:       int(row.Attempts),
		MaxAttempts:    int(row.MaxAttempts),
		IdempotencyKey: row.IdempotencyKey,
		LastError:      row.LastError,
	}
	s.logJobEvent(ctx, slog.LevelInfo, slog.LevelDebug, job.JobType, "job_claimed", observability.LogAttrs(ctx,
		"job_id", job.ID,
		"job_type", job.JobType,
		"worker", workerName,
		"run_at", job.RunAt.Format(time.RFC3339),
		"attempts", job.Attempts,
	)...)
	return job, true, nil
}

func (s *Store) MarkJobDone(ctx context.Context, jobID int64) error {
	jobType := ""
	if v, ok := ctx.Value(jobTypeCtxKey{}).(string); ok {
		jobType = v
	}
	err := s.queries.MarkJobDone(ctx, jobID)
	if err == nil {
		s.logJobEvent(ctx, slog.LevelInfo, slog.LevelDebug, jobType, "job_marked_done", observability.LogAttrs(ctx, "job_id", jobID)...)
	}
	return err
}

func (s *Store) MarkJobRetry(ctx context.Context, jobID int64, runAt time.Time, lastErr string) error {
	err := s.queries.MarkJobRetry(ctx, sqlcgen.MarkJobRetryParams{
		ID:        jobID,
		RunAt:     pgTimestamptzFromTime(runAt),
		LastError: emptyToNil(truncateForDB(lastErr, 1000)),
	})
	if err == nil {
		s.logger.WarnContext(ctx, "job_marked_retry", observability.LogAttrs(ctx,
			"job_id", jobID,
			"run_at", runAt.Format(time.RFC3339),
			"last_error", truncateForDB(lastErr, 200),
		)...)
	}
	return err
}

type jobTypeCtxKey struct{}

func WithJobType(ctx context.Context, jobType string) context.Context {
	return context.WithValue(ctx, jobTypeCtxKey{}, jobType)
}

func (s *Store) logJobEvent(ctx context.Context, normalLevel, deleteMessagesLevel slog.Level, jobType, msg string, attrs ...any) {
	level := normalLevel
	if jobType == domain.JobTypeDeleteMessages {
		level = deleteMessagesLevel
	}
	switch level {
	case slog.LevelDebug:
		s.logger.DebugContext(ctx, msg, attrs...)
	case slog.LevelWarn:
		s.logger.WarnContext(ctx, msg, attrs...)
	default:
		s.logger.InfoContext(ctx, msg, attrs...)
	}
}
