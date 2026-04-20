package bot

import (
	"context"
	"log/slog"
	"time"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/ui"
)

type Service struct {
	store           *postgres.Store
	tg              *telegram.Client
	ui              *ui.Renderer
	logger          *slog.Logger
	defaultTimezone string
	digestLocalTime string
}

func NewService(store *postgres.Store, tg *telegram.Client, copy *copycat.Loader, logger *slog.Logger, defaultTimezone, digestLocalTime string) *Service {
	return &Service{
		store:           store,
		tg:              tg,
		ui:              ui.New(copy),
		logger:          logger,
		defaultTimezone: defaultTimezone,
		digestLocalTime: digestLocalTime,
	}
}

func (s *Service) currentNow(ctx context.Context) (time.Time, error) {
	return s.store.CurrentNow(ctx, time.Now().UTC())
}

func (s *Service) enqueueJobNow(ctx context.Context, traceID, jobType string, payload any, idempotencyKey *string) error {
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	return s.store.EnqueueJob(ctx, traceID, jobType, payload, now, idempotencyKey)
}

func (s *Service) scheduleDeleteMessages(ctx context.Context, traceID string, chatID int64, delay time.Duration, messageIDs ...int64) error {
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	return s.store.EnqueueJob(ctx, traceID, domain.JobTypeDeleteMessages, jobs.DeleteMessagesPayload{
		TraceID:    traceID,
		ChatID:     chatID,
		MessageIDs: jobs.CompactMessageIDs(messageIDs...),
	}, now.Add(delay), nil)
}
