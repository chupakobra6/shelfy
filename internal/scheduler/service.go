package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/ui"
)

type TelegramAPI interface {
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	SendMessage(ctx context.Context, request telegram.SendMessageRequest) (telegram.Message, error)
}

type Store interface {
	SetClockOverride(ctx context.Context, value *time.Time) error
	AdvanceClock(ctx context.Context, delta time.Duration) (time.Time, error)
	CurrentNow(ctx context.Context, fallback time.Time) (time.Time, error)
	ListUsers(ctx context.Context) ([]postgres.UserSettings, error)
	ListVisibleProducts(ctx context.Context, userID int64, mode string, now time.Time) ([]domain.Product, error)
	CreateDigestMessage(ctx context.Context, userID, telegramMessageID int64, productIDs []int64) error
	ListActiveDigestMessages(ctx context.Context) ([]postgres.DigestMessage, error)
	MarkDigestDeleted(ctx context.Context, digestID int64) error
	ActiveProductsExist(ctx context.Context, productIDs []int64) (bool, error)
	GetUserSettings(ctx context.Context, userID int64) (postgres.UserSettings, error)
	EnqueueJob(ctx context.Context, traceID, jobType string, payload any, runAt time.Time, idempotencyKey *string) error
	CountActiveJobsUpTo(ctx context.Context, jobTypes []string, now time.Time) (int64, error)
	UpdateDraftStatus(ctx context.Context, draftID int64, status domain.DraftStatus) error
	ListStaleDrafts(ctx context.Context, now time.Time) ([]domain.DraftSession, error)
	ResetE2EUserState(ctx context.Context, userID int64, defaultTimezone, digestLocalTime string) (postgres.E2EResetResult, error)
}

type Options struct {
	DefaultTimezone string
	DigestLocalTime string
	E2ETestUserID   int64
	EnableE2EReset  bool
}

type Service struct {
	store           Store
	tg              TelegramAPI
	ui              *ui.Renderer
	logger          *slog.Logger
	defaultTimezone string
	digestLocalTime string
	e2eTestUserID   int64
	enableE2EReset  bool
}

func NewService(store *postgres.Store, tg *telegram.Client, copy *copycat.Loader, logger *slog.Logger, options Options) *Service {
	return &Service{
		store:           store,
		tg:              tg,
		ui:              ui.New(copy),
		logger:          logger,
		defaultTimezone: options.DefaultTimezone,
		digestLocalTime: options.DigestLocalTime,
		e2eTestUserID:   options.E2ETestUserID,
		enableE2EReset:  options.EnableE2EReset,
	}
}

func (s *Service) AllowedTypes() []string {
	return []string{domain.JobTypeDeleteMessages, domain.JobTypeSendMorningDigest}
}

func (s *Service) ProcessJob(ctx context.Context, job jobs.Envelope) error {
	s.logger.InfoContext(ctx, "scheduler_job_started", observability.LogAttrs(ctx, "job_type", job.JobType)...)
	switch job.JobType {
	case domain.JobTypeDeleteMessages:
		var payload jobs.DeleteMessagesPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		var firstErr error
		var failedMessageIDs []int64
		for _, messageID := range payload.MessageIDs {
			if err := s.tg.DeleteMessage(ctx, payload.ChatID, messageID); err != nil {
				if telegram.IsMissingMessageTargetError(err) {
					continue
				}
				failedMessageIDs = append(failedMessageIDs, messageID)
				if firstErr == nil {
					firstErr = err
				}
				s.logger.WarnContext(ctx, "delete_message_job_retryable_failure", observability.LogAttrs(ctx,
					"origin", payload.Origin,
					"chat_id", payload.ChatID,
					"message_id", messageID,
					"error", err,
				)...)
			}
		}
		if firstErr != nil {
			return fmt.Errorf("delete_messages retryable failures for origin %q chat %d message_ids %v: %w", payload.Origin, payload.ChatID, failedMessageIDs, firstErr)
		}
		s.logger.DebugContext(ctx, "delete_message_job_completed", observability.LogAttrs(ctx,
			"origin", payload.Origin,
			"chat_id", payload.ChatID,
			"message_count", len(payload.MessageIDs),
		)...)
		return nil
	case domain.JobTypeSendMorningDigest:
		var payload jobs.MorningDigestPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		return s.sendMorningDigest(ctx, payload)
	default:
		return fmt.Errorf("unsupported scheduler job type %s", job.JobType)
	}
}

func ptrValue(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
