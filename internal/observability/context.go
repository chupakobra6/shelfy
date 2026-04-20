package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey string

const (
	traceIDKey  ctxKey = "trace_id"
	updateIDKey ctxKey = "update_id"
	jobIDKey    ctxKey = "job_id"
	userIDKey   ctxKey = "user_id"
	draftIDKey  ctxKey = "draft_id"
)

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func TraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func EnsureTraceID(ctx context.Context) context.Context {
	if TraceID(ctx) != "" {
		return ctx
	}
	return WithTraceID(ctx, NewTraceID())
}

func NewTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "trace-fallback"
	}
	return hex.EncodeToString(buf)
}

func WithUpdateID(ctx context.Context, updateID int64) context.Context {
	return context.WithValue(ctx, updateIDKey, updateID)
}

func UpdateID(ctx context.Context) int64 {
	if v, ok := ctx.Value(updateIDKey).(int64); ok {
		return v
	}
	return 0
}

func WithJobID(ctx context.Context, jobID int64) context.Context {
	return context.WithValue(ctx, jobIDKey, jobID)
}

func JobID(ctx context.Context) int64 {
	if v, ok := ctx.Value(jobIDKey).(int64); ok {
		return v
	}
	return 0
}

func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(userIDKey).(int64); ok {
		return v
	}
	return 0
}

func WithDraftID(ctx context.Context, draftID int64) context.Context {
	return context.WithValue(ctx, draftIDKey, draftID)
}

func DraftID(ctx context.Context) int64 {
	if v, ok := ctx.Value(draftIDKey).(int64); ok {
		return v
	}
	return 0
}

func LogAttrs(ctx context.Context, attrs ...any) []any {
	out := make([]any, 0, len(attrs)+10)
	if traceID := TraceID(ctx); traceID != "" {
		out = append(out, "trace_id", traceID)
	}
	if updateID := UpdateID(ctx); updateID != 0 {
		out = append(out, "update_id", updateID)
	}
	if jobID := JobID(ctx); jobID != 0 {
		out = append(out, "job_id", jobID)
	}
	if userID := UserID(ctx); userID != 0 {
		out = append(out, "user_id", userID)
	}
	if draftID := DraftID(ctx); draftID != 0 {
		out = append(out, "draft_id", draftID)
	}
	out = append(out, attrs...)
	return out
}
