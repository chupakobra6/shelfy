package postgres

import (
	"context"
	"encoding/json"

	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
)

type DigestMessage struct {
	ID                int64
	UserID            int64
	TelegramMessageID int64
	ProductIDs        []int64
}

func (s *Store) CreateDigestMessage(ctx context.Context, userID, telegramMessageID int64, productIDs []int64) error {
	encoded, err := json.Marshal(productIDs)
	if err != nil {
		return err
	}
	_, err = s.queries.CreateDigestMessage(ctx, sqlcgen.CreateDigestMessageParams{
		UserID:            userID,
		TelegramMessageID: telegramMessageID,
		ProductIds:        encoded,
	})
	if err == nil {
		s.logger.InfoContext(ctx, "digest_message_created", observability.LogAttrs(ctx,
			"user_id", userID,
			"telegram_message_id", telegramMessageID,
			"product_count", len(productIDs),
		)...)
	}
	return err
}

func (s *Store) ListActiveDigestMessages(ctx context.Context) ([]DigestMessage, error) {
	rows, err := s.queries.ListActiveDigestMessages(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]DigestMessage, 0, len(rows))
	for _, row := range rows {
		item, err := digestMessageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Store) MarkDigestDeleted(ctx context.Context, digestID int64) error {
	err := s.queries.MarkDigestDeleted(ctx, digestID)
	if err == nil {
		s.logger.InfoContext(ctx, "digest_marked_deleted", observability.LogAttrs(ctx, "digest_id", digestID)...)
	}
	return err
}
