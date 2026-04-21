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
	store            *postgres.Store
	tg               *telegram.Client
	ui               *ui.Renderer
	logger           *slog.Logger
	tmpDir           string
	ollamaBaseURL    string
	ollamaModel      string
	tesseractCommand string
	voskCommand      string
	voskModelPath    string
}

func NewService(store *postgres.Store, tg *telegram.Client, copy *copycat.Loader, logger *slog.Logger, tmpDir, ollamaBaseURL, ollamaModel, tesseractCommand, voskCommand, voskModelPath string) *Service {
	return &Service{
		store:            store,
		tg:               tg,
		ui:               ui.New(copy),
		logger:           logger,
		tmpDir:           tmpDir,
		ollamaBaseURL:    strings.TrimRight(ollamaBaseURL, "/"),
		ollamaModel:      ollamaModel,
		tesseractCommand: tesseractCommand,
		voskCommand:      voskCommand,
		voskModelPath:    voskModelPath,
	}
}

func (s *Service) AllowedTypes() []string {
	return []string{domain.JobTypeIngestText, domain.JobTypeIngestPhoto, domain.JobTypeIngestAudio}
}

func (s *Service) currentNow(ctx context.Context) (time.Time, error) {
	return s.store.CurrentNow(ctx, time.Now().UTC())
}

func (s *Service) currentLocalNow(ctx context.Context, userID int64) (time.Time, error) {
	now, err := s.currentNow(ctx)
	if err != nil {
		return time.Time{}, err
	}
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		location = time.UTC
	}
	return now.In(location), nil
}

func (s *Service) TryHandleTextFast(ctx context.Context, payload jobs.IngestPayload) (bool, error) {
	if payload.Kind != domain.MessageKindText {
		return false, nil
	}
	localNow, err := s.currentLocalNow(ctx, payload.UserID)
	if err != nil {
		return false, err
	}
	cleaned := normalizeFreeText(payload.Text)
	result := heuristicParse(cleaned, localNow)
	if result.Name == "" && result.ExpiresOn == nil {
		return false, nil
	}
	if shouldTryTextModel(cleaned, result) {
		return false, nil
	}
	s.logger.InfoContext(ctx, "fast_text_path_accepted", observability.LogAttrs(ctx,
		"has_name", result.Name != "",
		"has_expiry", result.ExpiresOn != nil,
		"source", result.Source,
		"confidence", result.Confidence,
	)...)
	return true, s.createDraftCard(ctx, payload, result)
}

func (s *Service) ProcessJob(ctx context.Context, job jobs.Envelope) error {
	var payload jobs.IngestPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	ctx = observability.WithUserID(ctx, payload.UserID)
	now, err := s.currentNow(ctx)
	if err != nil {
		return err
	}
	settings, err := s.store.GetUserSettings(ctx, payload.UserID)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		location = time.UTC
	}
	localNow := now.In(location)
	s.logger.InfoContext(ctx, "ingest_job_started", observability.LogAttrs(ctx,
		"job_type", job.JobType,
		"message_kind", payload.Kind,
		"message_id", payload.MessageID,
		"local_now", localNow.Format(time.RFC3339),
	)...)
	switch job.JobType {
	case domain.JobTypeIngestText:
		return s.handleText(ctx, payload, localNow)
	case domain.JobTypeIngestPhoto:
		return s.handlePhoto(ctx, payload, localNow)
	case domain.JobTypeIngestAudio:
		return s.handleAudio(ctx, payload, localNow)
	default:
		return fmt.Errorf("unsupported ingest job type %s", job.JobType)
	}
}
