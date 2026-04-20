package postgres

import (
	"context"
	"encoding/json"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
)

func (s *Store) SaveIngestEvent(ctx context.Context, traceID string, userID, chatID, messageID int64, kind domain.MessageKind, status, summary string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = s.queries.CreateIngestEvent(ctx, sqlcgen.CreateIngestEventParams{
		TraceID:     traceID,
		UserID:      &userID,
		ChatID:      &chatID,
		MessageID:   &messageID,
		MessageKind: string(kind),
		Status:      status,
		Summary:     emptyToNil(summary),
		Metadata:    encoded,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "ingest_event_saved", observability.LogAttrs(ctx,
			"message_id", messageID,
			"message_kind", kind,
			"status", status,
		)...)
	}
	return err
}

func (s *Store) UpdateIngestStatus(ctx context.Context, traceID, status, summary string) error {
	err := s.queries.UpdateIngestStatus(ctx, sqlcgen.UpdateIngestStatusParams{
		TraceID: traceID,
		Status:  status,
		Summary: emptyToNil(summary),
	})
	if err == nil {
		s.logger.DebugContext(ctx, "ingest_status_updated", observability.LogAttrs(ctx,
			"status", status,
			"summary", truncateForDB(summary, 200),
		)...)
	}
	return err
}
