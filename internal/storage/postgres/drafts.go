package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateDraftSession(ctx context.Context, draft domain.DraftSession) (int64, error) {
	if draft.DraftPayload == nil {
		draft.DraftPayload = map[string]any{}
	}
	encoded, err := json.Marshal(draft.DraftPayload)
	if err != nil {
		return 0, err
	}
	id, err := s.queries.CreateDraftSession(ctx, sqlcgen.CreateDraftSessionParams{
		TraceID:           draft.TraceID,
		UserID:            draft.UserID,
		ChatID:            draft.ChatID,
		SourceKind:        string(draft.SourceKind),
		Status:            string(draft.Status),
		SourceMessageID:   draft.SourceMessageID,
		FeedbackMessageID: draft.FeedbackMessageID,
		DraftName:         emptyToNil(draft.DraftName),
		DraftExpiresOn:    pgDateFromTimePtr(draft.DraftExpiresOn),
		RawDeadlinePhrase: emptyToNil(draft.RawDeadlinePhrase),
		DraftPayload:      encoded,
		CleanupAfter:      pgTimestamptzFromTimePtr(draft.CleanupAfter),
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_session_created", observability.LogAttrs(ctx,
			"draft_id", id,
			"source_kind", draft.SourceKind,
			"status", draft.Status,
		)...)
	}
	return id, err
}

func (s *Store) GetDraftSession(ctx context.Context, draftID int64) (domain.DraftSession, error) {
	row, err := s.queries.GetDraftSession(ctx, draftID)
	if err != nil {
		return domain.DraftSession{}, err
	}
	return draftFromModel(row)
}

func (s *Store) FindDraftSessionByTraceID(ctx context.Context, traceID string) (domain.DraftSession, bool, error) {
	row, err := s.queries.GetDraftSessionByTraceID(ctx, traceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DraftSession{}, false, nil
		}
		return domain.DraftSession{}, false, err
	}
	item, err := draftFromModel(row)
	return item, err == nil, err
}

func (s *Store) FindEditableDraft(ctx context.Context, userID int64) (domain.DraftSession, bool, error) {
	row, err := s.queries.GetEditableDraftSessionForUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DraftSession{}, false, nil
		}
		return domain.DraftSession{}, false, err
	}
	item, err := draftFromModel(row)
	return item, err == nil, err
}

func (s *Store) SetDraftMessageID(ctx context.Context, draftID, messageID int64) error {
	err := s.queries.SetDraftMessageID(ctx, sqlcgen.SetDraftMessageIDParams{
		ID:             draftID,
		DraftMessageID: &messageID,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_message_id_set", observability.LogAttrs(ctx, "draft_id", draftID, "message_id", messageID)...)
	}
	return err
}

func (s *Store) SetDraftEditPromptMessageID(ctx context.Context, draftID int64, messageID *int64) error {
	err := s.queries.SetDraftEditPromptMessageID(ctx, sqlcgen.SetDraftEditPromptMessageIDParams{
		ID:                  draftID,
		EditPromptMessageID: messageID,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_edit_prompt_message_id_set", observability.LogAttrs(ctx, "draft_id", draftID, "message_id", optionalInt64(messageID))...)
	}
	return err
}

func (s *Store) UpdateDraftStatus(ctx context.Context, draftID int64, status domain.DraftStatus) error {
	err := s.queries.UpdateDraftStatus(ctx, sqlcgen.UpdateDraftStatusParams{
		ID:     draftID,
		Status: string(status),
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_status_updated", observability.LogAttrs(ctx, "draft_id", draftID, "status", status)...)
	}
	return err
}

func (s *Store) UpdateDraftFields(ctx context.Context, draftID int64, name string, expiresOn *time.Time, rawDeadline string, status domain.DraftStatus) error {
	err := s.queries.UpdateDraftFields(ctx, sqlcgen.UpdateDraftFieldsParams{
		ID:                draftID,
		DraftName:         emptyToNil(name),
		DraftExpiresOn:    pgDateFromTimePtr(expiresOn),
		RawDeadlinePhrase: emptyToNil(rawDeadline),
		Status:            string(status),
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_fields_updated", observability.LogAttrs(ctx, "draft_id", draftID, "status", status)...)
	}
	return err
}

func (s *Store) UpdateDraftPayload(ctx context.Context, draftID int64, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	err = s.queries.UpdateDraftPayload(ctx, sqlcgen.UpdateDraftPayloadParams{
		ID:           draftID,
		DraftPayload: encoded,
	})
	if err == nil {
		s.logger.DebugContext(ctx, "draft_payload_updated", observability.LogAttrs(ctx, "draft_id", draftID)...)
	}
	return err
}

func (s *Store) ApplyCleanerUpdateIfReady(ctx context.Context, draftID int64, name string, expiresOn *time.Time, rawDeadline string, payload map[string]any) (bool, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE draft_sessions
SET draft_name = $2,
    draft_expires_on = $3,
    raw_deadline_phrase = $4,
    draft_payload = $5,
    updated_at = NOW()
WHERE id = $1
  AND status = 'ready'
`, draftID, emptyToNil(name), pgDateFromTimePtr(expiresOn), emptyToNil(rawDeadline), encoded)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		s.logger.DebugContext(ctx, "cleaner_update_applied", observability.LogAttrs(ctx, "draft_id", draftID)...)
		return true, nil
	}
	return false, nil
}

func (s *Store) ListStaleDrafts(ctx context.Context, now time.Time) ([]domain.DraftSession, error) {
	rows, err := s.queries.ListStaleDraftSessions(ctx, pgTimestamptzFromTime(now.Add(-12*time.Hour)))
	if err != nil {
		return nil, err
	}
	result := make([]domain.DraftSession, 0, len(rows))
	for _, row := range rows {
		item, err := draftFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
