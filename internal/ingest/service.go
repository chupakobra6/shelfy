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
	return []string{domain.JobTypeIngestText, domain.JobTypeIngestAudio, domain.JobTypeRefineDraftAI}
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
	result, err := parseFastDraft(payload.Text, localNow)
	if err != nil {
		return false, nil
	}
	s.logger.InfoContext(ctx, "fast_text_path_accepted", observability.LogAttrs(ctx,
		"has_name", result.Name != "",
		"has_expiry", result.ExpiresOn != nil,
		"source", result.Source,
		"confidence", result.Confidence,
	)...)
	meta := buildDraftPayload(nil, result, payload, "", "")
	draft, err := s.upsertDraftCard(ctx, payload, result, meta)
	if err != nil {
		return false, err
	}
	if err := s.finishDraftReady(ctx, payload, "draft created"); err != nil {
		return false, err
	}
	s.startSmartReview(ctx, payload, draft, "", "")
	return true, nil
}

func (s *Service) ProcessJob(ctx context.Context, job jobs.Envelope) error {
	switch job.JobType {
	case domain.JobTypeIngestText:
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
		return s.handleText(ctx, payload, localNow)
	case domain.JobTypeIngestAudio:
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
		return s.handleAudio(ctx, payload, localNow)
	case domain.JobTypeRefineDraftAI:
		var payload jobs.RefineDraftAIPayload
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
			"message_kind", payload.SourceKind,
			"trace_id", payload.TraceID,
			"local_now", localNow.Format(time.RFC3339),
		)...)
		return s.handleRefineDraftAI(ctx, job.Payload, localNow)
	default:
		return fmt.Errorf("unsupported ingest job type %s", job.JobType)
	}
}
