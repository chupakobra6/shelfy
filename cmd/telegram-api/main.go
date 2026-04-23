package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/igor/shelfy/internal/bootstrap"
	"github.com/igor/shelfy/internal/bot"
	"github.com/igor/shelfy/internal/dispatcher"
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
	registerBotCommands(ctx, tg, runtime.Logger)
	fastText := ingest.NewService(
		runtime.Store,
		tg,
		runtime.Copy,
		runtime.Logger,
		runtime.Config.TmpDir,
		runtime.Config.OllamaBaseURL,
		runtime.Config.OllamaModel,
		runtime.Config.VoskCommand,
		runtime.Config.VoskModelPath,
		runtime.Config.VoskGrammarPath,
	)
	service := bot.NewService(runtime.Store, tg, runtime.Copy, runtime.Logger, runtime.Config.DefaultTimezone, runtime.Config.DigestLocalTime, fastText)
	updateDispatcher := dispatcher.New(32, 5*time.Minute)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := updateDispatcher.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			runtime.Logger.WarnContext(context.Background(), "update_dispatcher_shutdown_failed", "error", err)
		}
	}()

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
			update := update
			updateCtx := observability.EnsureTraceID(ctx)
			updateCtx = observability.WithUpdateID(updateCtx, update.UpdateID)
			if update.Message != nil && update.Message.From != nil {
				updateCtx = observability.WithUserID(updateCtx, update.Message.From.ID)
			}
			if update.CallbackQuery != nil {
				updateCtx = observability.WithUserID(updateCtx, update.CallbackQuery.From.ID)
			}
			tg.LogIncomingUpdate(updateCtx, update)
			if update.CallbackQuery != nil {
				if err := tg.AnswerCallbackQuery(updateCtx, telegram.AnswerCallbackQueryRequest{
					CallbackQueryID: update.CallbackQuery.ID,
				}); err != nil {
					runtime.Logger.WarnContext(updateCtx, "answer_callback_failed", "update_id", update.UpdateID, "error", err)
				}
			}
			if err := updateDispatcher.Submit(updateCtx, updateMailboxKey(update), func(jobCtx context.Context) {
				handleUpdate(jobCtx, runtime.Logger, service, update)
			}); err != nil {
				if errors.Is(err, dispatcher.ErrDispatcherClosed) || errors.Is(err, context.Canceled) {
					return
				}
				runtime.Logger.ErrorContext(updateCtx, "dispatch_update_failed", "update_id", update.UpdateID, "error", err)
			}
		}
	}
}

func handleUpdate(ctx context.Context, logger *slog.Logger, service *bot.Service, update telegram.Update) {
	if update.Message != nil && update.Message.From != nil {
		switch {
		case isTelegramCommand(update.Message.Text, "/start"):
			if err := service.HandleStart(ctx, update.Message.From.ID, update.Message.Chat.ID, update.Message.MessageID); err != nil {
				logger.ErrorContext(ctx, "handle_start_failed", "update_id", update.UpdateID, "error", err)
			}
			return
		case isTelegramCommand(update.Message.Text, "/dashboard"):
			if err := service.HandleDashboardCommand(ctx, update.Message.From.ID, update.Message.Chat.ID, update.Message.MessageID); err != nil {
				logger.ErrorContext(ctx, "handle_dashboard_command_failed", "update_id", update.UpdateID, "error", err)
			}
			return
		}
	}
	if update.Message != nil {
		if err := service.HandleMessage(ctx, *update.Message); err != nil {
			logger.ErrorContext(ctx, "handle_message_failed", "update_id", update.UpdateID, "error", err)
		}
		return
	}
	if update.CallbackQuery != nil {
		if err := service.HandleCallback(ctx, *update.CallbackQuery); err != nil {
			logger.ErrorContext(ctx, "handle_callback_failed", "update_id", update.UpdateID, "error", err)
		}
	}
}

func registerBotCommands(ctx context.Context, tg *telegram.Client, logger *slog.Logger) {
	if err := tg.SetMyCommands(ctx, []telegram.BotCommand{
		{
			Command:     "dashboard",
			Description: "Восстановить или обновить дашборд",
		},
	}); err != nil {
		logger.WarnContext(ctx, "register_bot_commands_failed", "error", err)
	}
}

func isTelegramCommand(text, command string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return false
	}
	token := strings.SplitN(fields[0], "@", 2)[0]
	return token == command
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

func updateMailboxKey(update telegram.Update) string {
	if update.CallbackQuery != nil {
		return strconv.FormatInt(update.CallbackQuery.From.ID, 10)
	}
	if update.Message != nil {
		if update.Message.From != nil {
			return strconv.FormatInt(update.Message.From.ID, 10)
		}
		return fmt.Sprintf("chat:%d", update.Message.Chat.ID)
	}
	return fmt.Sprintf("update:%d", update.UpdateID)
}
