package bot

import (
	"context"
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

type TextFastPath interface {
	TryHandleTextFast(ctx context.Context, payload jobs.IngestPayload) (bool, error)
}

type BotStore interface {
	CurrentNow(ctx context.Context, fallback time.Time) (time.Time, error)
	EnqueueJob(ctx context.Context, traceID, jobType string, payload any, runAt time.Time, idempotencyKey *string) error
	FindEditableDraft(ctx context.Context, userID int64) (domain.DraftSession, bool, error)
	GetUserSettings(ctx context.Context, userID int64) (postgres.UserSettings, error)
	UpsertUserSettings(ctx context.Context, settings postgres.UserSettings) error
	DashboardStats(ctx context.Context, userID int64, now time.Time) (domain.DashboardStats, error)
	SetDashboardMessageID(ctx context.Context, userID, messageID int64) error
	ListVisibleProducts(ctx context.Context, userID int64, mode string, now time.Time) ([]domain.Product, error)
	ListVisibleProductsPage(ctx context.Context, userID int64, mode string, now time.Time, limit, offset int) ([]domain.Product, int, error)
	CreateProductFromDraft(ctx context.Context, draftID int64) (domain.Product, error)
	GetDraftSession(ctx context.Context, draftID int64) (domain.DraftSession, error)
	UpdateDraftStatus(ctx context.Context, draftID int64, status domain.DraftStatus) error
	GetProduct(ctx context.Context, productID int64) (domain.Product, error)
	UpdateProductStatus(ctx context.Context, productID int64, status domain.ProductStatus) error
	UpdateUserDigestLocalTime(ctx context.Context, userID int64, digestLocalTime string) error
	UpdateUserTimezone(ctx context.Context, userID int64, timezone string) error
	SetDraftEditPromptMessageID(ctx context.Context, draftID int64, messageID *int64) error
	SaveIngestEvent(ctx context.Context, traceID string, userID, chatID, messageID int64, kind domain.MessageKind, status, summary string, metadata map[string]any) error
	UpdateDraftFields(ctx context.Context, draftID int64, name string, expiresOn *time.Time, rawDeadline string, status domain.DraftStatus) error
}

type TelegramAPI interface {
	SendMessage(ctx context.Context, request telegram.SendMessageRequest) (telegram.Message, error)
	EditMessageText(ctx context.Context, request telegram.EditMessageTextRequest) error
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
	PinMessage(ctx context.Context, chatID, messageID int64) error
	AnswerCallbackQuery(ctx context.Context, request telegram.AnswerCallbackQueryRequest) error
}

type Service struct {
	store           BotStore
	tg              TelegramAPI
	ops             *TelegramOps
	ui              *ui.Renderer
	logger          *slog.Logger
	defaultTimezone string
	digestLocalTime string
	textFastPath    TextFastPath
}

func NewService(store BotStore, tg TelegramAPI, copy *copycat.Loader, logger *slog.Logger, defaultTimezone, digestLocalTime string, textFastPath TextFastPath) *Service {
	service := &Service{
		store:           store,
		tg:              tg,
		ui:              ui.New(copy),
		logger:          logger,
		defaultTimezone: defaultTimezone,
		digestLocalTime: digestLocalTime,
		textFastPath:    textFastPath,
	}
	service.ops = NewTelegramOps(service)
	return service
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
	return s.scheduleDeleteMessagesWithOrigin(ctx, traceID, "", chatID, delay, messageIDs...)
}

func (s *Service) scheduleDeleteMessagesWithOrigin(ctx context.Context, traceID, origin string, chatID int64, delay time.Duration, messageIDs ...int64) error {
	compactIDs := jobs.CompactMessageIDs(messageIDs...)
	if len(compactIDs) == 0 {
		return nil
	}
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	if err := s.store.EnqueueJob(ctx, traceID, domain.JobTypeDeleteMessages, jobs.DeleteMessagesPayload{
		TraceID:    traceID,
		Origin:     origin,
		ChatID:     chatID,
		MessageIDs: compactIDs,
	}, now.Add(delay), nil); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "cleanup_fallback_scheduled", observability.LogAttrs(ctx,
		"origin", origin,
		"chat_id", chatID,
		"delay_ms", delay.Milliseconds(),
		"message_ids", compactIDs,
	)...)
	return nil
}

func (s *Service) deleteMessagesNow(ctx context.Context, origin string, chatID int64, messageIDs ...int64) {
	for _, messageID := range jobs.CompactMessageIDs(messageIDs...) {
		if err := s.tg.DeleteMessage(ctx, chatID, messageID); err != nil {
			if telegram.IsMissingMessageTargetError(err) {
				continue
			}
			s.logger.WarnContext(ctx, "cleanup_delete_now_failed", observability.LogAttrs(ctx,
				"origin", origin,
				"chat_id", chatID,
				"message_id", messageID,
				"error", err,
			)...)
		}
	}
}

func (s *Service) deleteMessagesReliably(ctx context.Context, traceID, origin string, chatID int64, delay time.Duration, messageIDs ...int64) error {
	compactIDs := jobs.CompactMessageIDs(messageIDs...)
	if len(compactIDs) == 0 {
		return nil
	}
	s.deleteMessagesNow(ctx, origin, chatID, compactIDs...)
	return s.scheduleDeleteMessagesWithOrigin(ctx, traceID, origin, chatID, delay, compactIDs...)
}
