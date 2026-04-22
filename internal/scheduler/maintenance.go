package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/telegram"
)

func (s *Service) RunMaintenanceTick(ctx context.Context) error {
	now, err := s.store.CurrentNow(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	s.logger.DebugContext(ctx, "scheduler_maintenance_started", observability.LogAttrs(ctx, "now", now.Format(time.RFC3339))...)
	if err := s.enqueueDueDigests(ctx, now); err != nil {
		return err
	}
	if err := s.cleanupDigestMessages(ctx); err != nil {
		return err
	}
	if err := s.cleanupStaleDrafts(ctx, now); err != nil {
		return err
	}
	s.logger.DebugContext(ctx, "scheduler_maintenance_completed", observability.LogAttrs(ctx, "now", now.Format(time.RFC3339))...)
	return nil
}

func (s *Service) enqueueDueDigests(ctx context.Context, now time.Time) error {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		localNow := domain.LocalizeTime(now, user.Timezone)
		if localNow.Format("15:04") != user.DigestLocalTime {
			continue
		}
		key := fmt.Sprintf("digest:%d:%s", user.UserID, localNow.Format("2006-01-02"))
		payload := jobs.MorningDigestPayload{
			TraceID: key,
			UserID:  user.UserID,
			ChatID:  user.ChatID,
		}
		if err := s.store.EnqueueJob(ctx, key, domain.JobTypeSendMorningDigest, payload, now, &key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sendMorningDigest(ctx context.Context, payload jobs.MorningDigestPayload) error {
	now, err := s.store.CurrentNow(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	soon, err := s.store.ListVisibleProducts(ctx, payload.UserID, "soon", now)
	if err != nil {
		return err
	}
	expired, err := s.store.ListVisibleProducts(ctx, payload.UserID, "expired", now)
	if err != nil {
		return err
	}
	lines := []string{}
	productIDs := []int64{}
	for _, item := range expired {
		line, err := s.ui.DigestLine("expired", item.Name, item.ExpiresOn)
		if err != nil {
			return err
		}
		lines = append(lines, line)
		productIDs = append(productIDs, item.ID)
	}
	for _, item := range soon {
		line, err := s.ui.DigestLine("soon", item.Name, item.ExpiresOn)
		if err != nil {
			return err
		}
		lines = append(lines, line)
		productIDs = append(productIDs, item.ID)
	}
	if len(lines) == 0 {
		return nil
	}
	text, err := s.ui.DigestMessage(lines)
	if err != nil {
		return err
	}
	message, err := s.tg.SendMessage(ctx, telegram.SendMessageRequest{
		ChatID:    payload.ChatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	return s.store.CreateDigestMessage(ctx, payload.UserID, message.MessageID, productIDs)
}

func (s *Service) cleanupDigestMessages(ctx context.Context) error {
	digests, err := s.store.ListActiveDigestMessages(ctx)
	if err != nil {
		return err
	}
	for _, digest := range digests {
		exists, err := s.store.ActiveProductsExist(ctx, digest.ProductIDs)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		settings, err := s.store.GetUserSettings(ctx, digest.UserID)
		if err != nil {
			return err
		}
		if err := s.tg.DeleteMessage(ctx, settings.ChatID, digest.TelegramMessageID); !shouldMarkDigestDeletedAfterCleanup(err) {
			s.logger.WarnContext(ctx, "digest_cleanup_deferred", observability.LogAttrs(ctx,
				"digest_id", digest.ID,
				"user_id", digest.UserID,
				"telegram_message_id", digest.TelegramMessageID,
				"error", err,
			)...)
			continue
		}
		if err := s.store.MarkDigestDeleted(ctx, digest.ID); err != nil {
			return err
		}
	}
	return nil
}

func shouldMarkDigestDeletedAfterCleanup(err error) bool {
	return err == nil || telegram.IsMissingMessageTargetError(err)
}

func (s *Service) cleanupStaleDrafts(ctx context.Context, now time.Time) error {
	drafts, err := s.store.ListStaleDrafts(ctx, now)
	if err != nil {
		return err
	}
	for _, draft := range drafts {
		key := fmt.Sprintf("draft-cleanup:%d", draft.ID)
		payload := jobs.DeleteMessagesPayload{
			TraceID:    draft.TraceID,
			ChatID:     draft.ChatID,
			MessageIDs: jobs.CompactMessageIDs(ptrValue(draft.SourceMessageID), ptrValue(draft.DraftMessageID), ptrValue(draft.FeedbackMessageID), ptrValue(draft.EditPromptMessageID)),
		}
		if err := s.store.EnqueueJob(ctx, draft.TraceID, domain.JobTypeDeleteMessages, payload, now, &key); err != nil {
			return err
		}
		if err := s.store.UpdateDraftStatus(ctx, draft.ID, domain.DraftStatusFailed); err != nil {
			return err
		}
	}
	return nil
}
