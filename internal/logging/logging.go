package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

func New(level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "WARN":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}

func With(ctx context.Context, logger *slog.Logger, attrs ...any) context.Context {
	return context.WithValue(ctx, contextLoggerKey{}, logger.With(attrs...))
}

func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	logger, ok := ctx.Value(contextLoggerKey{}).(*slog.Logger)
	if ok && logger != nil {
		return logger
	}
	return fallback
}

type contextLoggerKey struct{}
