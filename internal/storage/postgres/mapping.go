package postgres

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgtype"
)

func emptyToNil(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeName(v string) string {
	return strings.Join(strings.Fields(strings.ToLower(v)), " ")
}

func truncateForDB(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit]
}

func pgDateFromTime(t time.Time) pgtype.Date {
	return pgtype.Date{Time: truncateDate(t), Valid: true}
}

func pgDateFromTimePtr(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgDateFromTime(*t)
}

func pgTimestamptzFromTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func pgTimestamptzFromTimePtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgTimestamptzFromTime(*t)
}

func timeFromPgDate(value pgtype.Date) time.Time {
	return truncateDate(value.Time)
}

func timePtrFromPgDate(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	result := timeFromPgDate(value)
	return &result
}

func timeFromPgTimestamptz(value pgtype.Timestamptz) time.Time {
	return value.Time.UTC()
}

func timePtrFromPgTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := timeFromPgTimestamptz(value)
	return &result
}

func truncateDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func productFromModel(item sqlcgen.Product) domain.Product {
	return domain.Product{
		ID:                item.ID,
		UserID:            item.UserID,
		Name:              item.Name,
		NormalizedName:    item.NormalizedName,
		ExpiresOn:         timeFromPgDate(item.ExpiresOn),
		RawDeadlinePhrase: ptrStringValue(item.RawDeadlinePhrase),
		Status:            domain.ProductStatus(item.Status),
		SourceKind:        domain.MessageKind(item.SourceKind),
		CreatedAt:         timeFromPgTimestamptz(item.CreatedAt),
		ClosedAt:          timePtrFromPgTimestamptz(item.ClosedAt),
	}
}

func productsFromModels(items []sqlcgen.Product) []domain.Product {
	result := make([]domain.Product, 0, len(items))
	for _, item := range items {
		result = append(result, productFromModel(item))
	}
	return result
}

func draftFromModel(item sqlcgen.DraftSession) (domain.DraftSession, error) {
	payload := map[string]any{}
	if len(item.DraftPayload) > 0 {
		if err := json.Unmarshal(item.DraftPayload, &payload); err != nil {
			return domain.DraftSession{}, err
		}
	}
	return domain.DraftSession{
		ID:                  item.ID,
		TraceID:             item.TraceID,
		UserID:              item.UserID,
		ChatID:              item.ChatID,
		SourceKind:          domain.MessageKind(item.SourceKind),
		Status:              domain.DraftStatus(item.Status),
		SourceMessageID:     item.SourceMessageID,
		DraftMessageID:      item.DraftMessageID,
		FeedbackMessageID:   item.FeedbackMessageID,
		EditPromptMessageID: item.EditPromptMessageID,
		ConfirmedProductID:  item.ConfirmedProductID,
		DraftName:           ptrStringValue(item.DraftName),
		DraftExpiresOn:      timePtrFromPgDate(item.DraftExpiresOn),
		RawDeadlinePhrase:   ptrStringValue(item.RawDeadlinePhrase),
		DraftPayload:        payload,
		CleanupAfter:        timePtrFromPgTimestamptz(item.CleanupAfter),
		CreatedAt:           timeFromPgTimestamptz(item.CreatedAt),
		UpdatedAt:           timeFromPgTimestamptz(item.UpdatedAt),
	}, nil
}

func digestMessageFromModel(item sqlcgen.DigestMessage) (DigestMessage, error) {
	var productIDs []int64
	if len(item.ProductIds) > 0 {
		if err := json.Unmarshal(item.ProductIds, &productIDs); err != nil {
			return DigestMessage{}, err
		}
	}
	return DigestMessage{
		ID:                item.ID,
		UserID:            item.UserID,
		TelegramMessageID: item.TelegramMessageID,
		ProductIDs:        productIDs,
	}, nil
}

func ptrStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func optionalInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
