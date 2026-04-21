package bot

import (
	"context"
	"time"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

type TelegramOps struct {
	service *Service
}

func NewTelegramOps(service *Service) *TelegramOps {
	return &TelegramOps{service: service}
}

func (o *TelegramOps) DeleteMessagesReliably(ctx context.Context, traceID, origin string, chatID int64, delay time.Duration, messageIDs ...int64) error {
	return o.service.deleteMessagesReliably(ctx, traceID, origin, chatID, delay, messageIDs...)
}

func (o *TelegramOps) SendTransientFeedback(ctx context.Context, chatID int64, text string, delay time.Duration) error {
	message, err := o.service.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	traceID := observability.TraceID(observability.EnsureTraceID(ctx))
	return o.service.scheduleDeleteMessages(ctx, traceID, chatID, delay, message.MessageID)
}

func (o *TelegramOps) CreateDashboard(ctx context.Context, userID, chatID int64, state dashboardState) (telegram.Message, dashboardState, error) {
	text, markup, effectiveState, err := o.service.renderDashboardState(ctx, userID, state)
	if err != nil {
		return telegram.Message{}, homeDashboardState(), err
	}
	message, err := o.service.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if err != nil {
		return telegram.Message{}, homeDashboardState(), err
	}
	if err := o.service.store.SetDashboardMessageID(ctx, userID, message.MessageID); err != nil {
		return telegram.Message{}, homeDashboardState(), err
	}
	if err := o.service.tg.PinMessage(ctx, chatID, message.MessageID); err != nil {
		return telegram.Message{}, homeDashboardState(), err
	}
	return message, effectiveState, nil
}

func (o *TelegramOps) ApplyDashboard(ctx context.Context, userID, chatID, messageID int64, state dashboardState) (dashboardState, error) {
	text, markup, effectiveState, err := o.service.renderDashboardState(ctx, userID, state)
	if err != nil {
		return homeDashboardState(), err
	}
	if err := o.service.tg.EditMessageText(ctx, telegram.EditMessageTextRequest{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	}); err != nil {
		return homeDashboardState(), err
	}
	return effectiveState, nil
}
