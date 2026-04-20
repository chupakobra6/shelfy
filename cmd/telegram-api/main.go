package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/igor/shelfy/internal/bootstrap"
	"github.com/igor/shelfy/internal/bot"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := bootstrap.Load(ctx, true)
	if err != nil {
		panic(err)
	}
	defer runtime.Close()
	tg := telegram.NewClient(runtime.Config.BotToken, runtime.Logger)
	service := bot.NewService(runtime.Store, tg, runtime.Copy, runtime.Logger, runtime.Config.DefaultTimezone, runtime.Config.DigestLocalTime)

	var offset int64
	for {
		updates, err := tg.PollUpdates(ctx, offset, runtime.Config.PollTimeoutSeconds)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			runtime.Logger.ErrorContext(ctx, "poll_updates_failed", "error", err)
			continue
		}
		if len(updates) > 0 {
			runtime.Logger.InfoContext(ctx, "telegram.poll_batch_received",
				"update_count", len(updates),
				"first_update_id", updates[0].UpdateID,
				"last_update_id", updates[len(updates)-1].UpdateID,
			)
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			updateCtx := observability.EnsureTraceID(context.Background())
			updateCtx = observability.WithUpdateID(updateCtx, update.UpdateID)
			if update.Message != nil && update.Message.From != nil {
				updateCtx = observability.WithUserID(updateCtx, update.Message.From.ID)
			}
			if update.CallbackQuery != nil {
				updateCtx = observability.WithUserID(updateCtx, update.CallbackQuery.From.ID)
			}
			tg.LogIncomingUpdate(updateCtx, update)
			if update.Message != nil && update.Message.Text == "/start" {
				if err := service.HandleStart(updateCtx, update.Message.From.ID, update.Message.Chat.ID); err != nil {
					runtime.Logger.ErrorContext(updateCtx, "handle_start_failed", "update_id", update.UpdateID, "error", err)
				}
				continue
			}
			if update.Message != nil {
				if err := service.HandleMessage(updateCtx, *update.Message); err != nil {
					runtime.Logger.ErrorContext(updateCtx, "handle_message_failed", "update_id", update.UpdateID, "error", err)
				}
				continue
			}
			if update.CallbackQuery != nil {
				if err := service.HandleCallback(updateCtx, *update.CallbackQuery); err != nil {
					runtime.Logger.ErrorContext(updateCtx, "handle_callback_failed", "update_id", update.UpdateID, "error", err)
				}
			}
		}
	}
}
