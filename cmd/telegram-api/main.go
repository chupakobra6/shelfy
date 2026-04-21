package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/igor/shelfy/internal/bootstrap"
	"github.com/igor/shelfy/internal/bot"
	"github.com/igor/shelfy/internal/ingest"
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
	fastText := ingest.NewService(
		runtime.Store,
		tg,
		runtime.Copy,
		runtime.Logger,
		runtime.Config.TmpDir,
		runtime.Config.OllamaBaseURL,
		runtime.Config.OllamaModel,
		runtime.Config.TesseractCommand,
		runtime.Config.WhisperCommand,
		runtime.Config.WhisperModelPath,
	)
	service := bot.NewService(runtime.Store, tg, runtime.Copy, runtime.Logger, runtime.Config.DefaultTimezone, runtime.Config.DigestLocalTime, fastText)

	var offset int64
	var pollErrorStreak int
	for {
		updates, err := tg.PollUpdates(ctx, offset, runtime.Config.PollTimeoutSeconds)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			pollErrorStreak++
			logPollError(runtime.Logger, ctx, err, pollErrorStreak)
			select {
			case <-time.After(pollRetryDelay(pollErrorStreak)):
			case <-ctx.Done():
				return
			}
			continue
		}
		pollErrorStreak = 0
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
				if err := service.HandleStart(updateCtx, update.Message.From.ID, update.Message.Chat.ID, update.Message.MessageID); err != nil {
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

func logPollError(logger *slog.Logger, ctx context.Context, err error, streak int) {
	attrs := []any{"error", err, "streak", streak}
	if telegram.IsTransientPollError(err) {
		if streak == 1 || streak%10 == 0 {
			logger.WarnContext(ctx, "poll_updates_transient_failure", attrs...)
			return
		}
		logger.DebugContext(ctx, "poll_updates_transient_failure", attrs...)
		return
	}
	if streak == 1 || streak%5 == 0 {
		logger.ErrorContext(ctx, "poll_updates_failed", attrs...)
		return
	}
	logger.DebugContext(ctx, "poll_updates_failed", attrs...)
}

func pollRetryDelay(streak int) time.Duration {
	switch {
	case streak <= 1:
		return time.Second
	case streak <= 5:
		return 2 * time.Second
	case streak <= 15:
		return 5 * time.Second
	default:
		return 15 * time.Second
	}
}
