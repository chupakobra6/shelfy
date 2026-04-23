package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/ui"
)

type Service struct {
	store           *postgres.Store
	tg              *telegram.Client
	ui              *ui.Renderer
	logger          *slog.Logger
	tmpDir          string
	ollamaBaseURL   string
	ollamaModel     string
	voskCommand     string
	voskModelPath   string
	voskGrammarPath string
}

func NewService(store *postgres.Store, tg *telegram.Client, copy *copycat.Loader, logger *slog.Logger, tmpDir, ollamaBaseURL, ollamaModel, voskCommand, voskModelPath, voskGrammarPath string) *Service {
	return &Service{
		store:           store,
		tg:              tg,
		ui:              ui.New(copy),
		logger:          logger,
		tmpDir:          tmpDir,
		ollamaBaseURL:   strings.TrimRight(ollamaBaseURL, "/"),
		ollamaModel:     ollamaModel,
		voskCommand:     voskCommand,
		voskModelPath:   voskModelPath,
		voskGrammarPath: voskGrammarPath,
	}
}

func (s *Service) AllowedTypes() []string {
	return []string{domain.JobTypeIngestText, domain.JobTypeIngestAudio, domain.JobTypeCleanDraft}
}

func (s *Service) currentNow(ctx context.Context) (time.Time, error) {
	return s.store.CurrentNow(ctx, time.Now().UTC())
}

func (s *Service) currentLocalNow(ctx context.Context, userID int64) (time.Time, error) {
	now, err := s.currentNow(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return s.localizeNowForUser(ctx, userID, now)
}

func (s *Service) localizeNowForUser(ctx context.Context, userID int64, now time.Time) (time.Time, error) {
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	return domain.LocalizeTime(now, settings.Timezone), nil
}

func (s *Service) TryHandleTextFast(ctx context.Context, payload jobs.IngestPayload) (bool, error) {
	if payload.Kind != domain.MessageKindText {
		return false, nil
	}
	localNow, err := s.currentLocalNow(ctx, payload.UserID)
	if err != nil {
		return false, err
	}
	result, err := buildInitialDraft(payload.Text, localNow)
	if err != nil {
		return false, nil
	}
	s.logger.InfoContext(ctx, "fast_text_path_accepted", observability.LogAttrs(ctx,
		"has_name", result.Draft.Name != "",
		"has_expiry", result.Draft.ExpiresOn != nil,
		"source", result.Draft.Source,
		"confidence", result.Draft.Confidence,
	)...)
	draft, err := s.persistReadyDraft(ctx, payload, result)
	if err != nil {
		return false, err
	}
	if err := s.enqueueBackgroundCleaner(ctx, payload, draft, result.Trace.NormalizedInput); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) ProcessJob(ctx context.Context, job jobs.Envelope) error {
	switch job.JobType {
	case domain.JobTypeIngestText:
		return s.processIngestPayloadJob(ctx, job, s.handleText)
	case domain.JobTypeIngestAudio:
		return s.processIngestPayloadJob(ctx, job, s.handleAudio)
	case domain.JobTypeCleanDraft:
		var payload jobs.CleanDraftPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		ctx = observability.WithUserID(ctx, payload.UserID)
		return s.handleCleanDraft(ctx, payload)
	default:
		return fmt.Errorf("unsupported ingest job type %s", job.JobType)
	}
}

func (s *Service) processIngestPayloadJob(ctx context.Context, job jobs.Envelope, handler func(context.Context, jobs.IngestPayload, time.Time) error) error {
	var payload jobs.IngestPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	ctx = observability.WithUserID(ctx, payload.UserID)
	localNow, err := s.currentLocalNow(ctx, payload.UserID)
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "ingest_job_started", observability.LogAttrs(ctx,
		"job_type", job.JobType,
		"message_kind", payload.Kind,
		"message_id", payload.MessageID,
		"local_now", localNow.Format(time.RFC3339),
	)...)
	return handler(ctx, payload, localNow)
}
