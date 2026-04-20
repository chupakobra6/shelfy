package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/ui"
)

type Service struct {
	store  *postgres.Store
	tg     *telegram.Client
	ui     *ui.Renderer
	logger *slog.Logger
}

func NewService(store *postgres.Store, tg *telegram.Client, copy *copycat.Loader, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		tg:     tg,
		ui:     ui.New(copy),
		logger: logger,
	}
}

func (s *Service) AllowedTypes() []string {
	return []string{domain.JobTypeDeleteMessages, domain.JobTypeSendMorningDigest}
}

func (s *Service) ProcessJob(ctx context.Context, job jobs.Envelope) error {
	s.logger.InfoContext(ctx, "scheduler_job_started", observability.LogAttrs(ctx, "job_type", job.JobType)...)
	switch job.JobType {
	case domain.JobTypeDeleteMessages:
		var payload jobs.DeleteMessagesPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		for _, messageID := range payload.MessageIDs {
			if err := s.tg.DeleteMessage(ctx, payload.ChatID, messageID); err != nil {
				s.logger.WarnContext(ctx, "delete_message_failed", observability.LogAttrs(ctx,
					"chat_id", payload.ChatID,
					"message_id", messageID,
					"error", err,
				)...)
			}
		}
		return nil
	case domain.JobTypeSendMorningDigest:
		var payload jobs.MorningDigestPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		return s.sendMorningDigest(ctx, payload)
	default:
		return fmt.Errorf("unsupported scheduler job type %s", job.JobType)
	}
}

func ptrValue(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
