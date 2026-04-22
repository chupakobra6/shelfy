package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/storage/postgres"
)

type fakeControlStore struct {
	resetResult          postgres.E2EResetResult
	resetErr             error
	resetCalls           int
	resetUserID          int64
	resetDefaultTZ       string
	resetDigestLocalTime string
	activeJobCounts      []int64
	activeJobCountCalls  int
}

func (s *fakeControlStore) SetClockOverride(context.Context, *time.Time) error {
	return nil
}

func (s *fakeControlStore) AdvanceClock(context.Context, time.Duration) (time.Time, error) {
	return time.Time{}, nil
}

func (s *fakeControlStore) CurrentNow(_ context.Context, fallback time.Time) (time.Time, error) {
	return fallback, nil
}

func (s *fakeControlStore) ListUsers(context.Context) ([]postgres.UserSettings, error) {
	return nil, nil
}

func (s *fakeControlStore) ListVisibleProducts(context.Context, int64, string, time.Time) ([]domain.Product, error) {
	return nil, nil
}

func (s *fakeControlStore) CreateDigestMessage(context.Context, int64, int64, []int64) error {
	return nil
}

func (s *fakeControlStore) ListActiveDigestMessages(context.Context) ([]postgres.DigestMessage, error) {
	return nil, nil
}

func (s *fakeControlStore) MarkDigestDeleted(context.Context, int64) error {
	return nil
}

func (s *fakeControlStore) ActiveProductsExist(context.Context, []int64) (bool, error) {
	return false, nil
}

func (s *fakeControlStore) GetUserSettings(context.Context, int64) (postgres.UserSettings, error) {
	return postgres.UserSettings{}, nil
}

func (s *fakeControlStore) EnqueueJob(context.Context, string, string, any, time.Time, *string) error {
	return nil
}

func (s *fakeControlStore) CountActiveJobsUpTo(context.Context, []string, time.Time) (int64, error) {
	if s.activeJobCountCalls >= len(s.activeJobCounts) {
		return 0, nil
	}
	value := s.activeJobCounts[s.activeJobCountCalls]
	s.activeJobCountCalls++
	return value, nil
}

func (s *fakeControlStore) UpdateDraftStatus(context.Context, int64, domain.DraftStatus) error {
	return nil
}

func (s *fakeControlStore) ListStaleDrafts(context.Context, time.Time) ([]domain.DraftSession, error) {
	return nil, nil
}

func (s *fakeControlStore) ResetE2EUserState(_ context.Context, userID int64, defaultTimezone, digestLocalTime string) (postgres.E2EResetResult, error) {
	s.resetCalls++
	s.resetUserID = userID
	s.resetDefaultTZ = defaultTimezone
	s.resetDigestLocalTime = digestLocalTime
	if s.resetErr != nil {
		return postgres.E2EResetResult{}, s.resetErr
	}
	return s.resetResult, nil
}

func newControlTestService(store Store, tg TelegramAPI, enable bool) *Service {
	return &Service{
		store:           store,
		tg:              tg,
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		defaultTimezone: "Europe/Moscow",
		digestLocalTime: "09:00",
		e2eTestUserID:   8031865593,
		enableE2EReset:  enable,
	}
}

func TestHandlerRegistersE2EResetOnlyWhenEnabled(t *testing.T) {
	disabled := newControlTestService(&fakeControlStore{}, &fakeTelegram{}, false)
	req := httptest.NewRequest(http.MethodGet, "/control/e2e/reset", nil)
	rec := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled reset handler code = %d, want %d", rec.Code, http.StatusNotFound)
	}

	enabled := newControlTestService(&fakeControlStore{}, &fakeTelegram{}, true)
	rec = httptest.NewRecorder()
	enabled.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("enabled reset handler code = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleE2EResetReturnsResultAndIgnoresDeleteFailures(t *testing.T) {
	store := &fakeControlStore{
		resetResult: postgres.E2EResetResult{
			UserID:                     8031865593,
			ChatID:                     8031865593,
			SettingsFound:              true,
			ClearedDashboardMessageID:  int64Ptr(3655),
			CleanupAttemptedMessageIDs: []int64{3655, 4001},
			Deleted: postgres.E2EResetDeletedCounts{
				Drafts:   2,
				Products: 4,
				Digests:  1,
				Jobs:     6,
			},
		},
	}
	tg := &fakeTelegram{
		deleteErrs: []error{
			errors.New("telegram deleteMessage not ok: Bad Request: message to delete not found"),
			errors.New("write tcp timeout"),
		},
	}
	service := newControlTestService(store, tg, true)

	req := httptest.NewRequest(http.MethodPost, "/control/e2e/reset", nil)
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset handler code = %d, want %d", rec.Code, http.StatusOK)
	}
	if store.resetCalls != 1 {
		t.Fatalf("reset calls = %d, want 1", store.resetCalls)
	}
	if store.resetUserID != 8031865593 {
		t.Fatalf("reset user id = %d, want 8031865593", store.resetUserID)
	}
	if store.resetDefaultTZ != "Europe/Moscow" || store.resetDigestLocalTime != "09:00" {
		t.Fatalf("reset defaults = %q/%q, want Europe/Moscow/09:00", store.resetDefaultTZ, store.resetDigestLocalTime)
	}
	if len(tg.deleteRequests) != 2 {
		t.Fatalf("delete requests = %v, want 2 cleanup attempts", tg.deleteRequests)
	}

	var body struct {
		Ok                         bool             `json:"ok"`
		UserID                     int64            `json:"user_id"`
		ChatID                     int64            `json:"chat_id"`
		SettingsFound              bool             `json:"settings_found"`
		ClearedDashboardMessageID  *int64           `json:"cleared_dashboard_message_id"`
		CleanupAttemptedMessageIDs []int64          `json:"cleanup_attempted_message_ids"`
		Deleted                    map[string]int64 `json:"deleted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !body.Ok || !body.SettingsFound {
		t.Fatalf("unexpected body flags: %+v", body)
	}
	if body.UserID != 8031865593 || body.ChatID != 8031865593 {
		t.Fatalf("unexpected ids: %+v", body)
	}
	if body.ClearedDashboardMessageID == nil || *body.ClearedDashboardMessageID != 3655 {
		t.Fatalf("cleared_dashboard_message_id = %v, want 3655", body.ClearedDashboardMessageID)
	}
	if len(body.CleanupAttemptedMessageIDs) != 2 {
		t.Fatalf("cleanup_attempted_message_ids = %v, want 2 ids", body.CleanupAttemptedMessageIDs)
	}
	if body.Deleted["jobs"] != 6 {
		t.Fatalf("deleted jobs = %d, want 6", body.Deleted["jobs"])
	}
}

func TestHandleE2EResetMissingSettingsStillReturnsOK(t *testing.T) {
	service := newControlTestService(&fakeControlStore{
		resetResult: postgres.E2EResetResult{
			UserID:        8031865593,
			SettingsFound: false,
		},
	}, &fakeTelegram{}, true)

	req := httptest.NewRequest(http.MethodPost, "/control/e2e/reset", nil)
	rec := httptest.NewRecorder()
	service.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset handler code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Ok            bool  `json:"ok"`
		SettingsFound bool  `json:"settings_found"`
		ChatID        int64 `json:"chat_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !body.Ok || body.SettingsFound {
		t.Fatalf("unexpected body = %+v", body)
	}
	if body.ChatID != 0 {
		t.Fatalf("chat_id = %d, want 0", body.ChatID)
	}
}

func TestWaitForSettledJobTypesPollsUntilZero(t *testing.T) {
	store := &fakeControlStore{activeJobCounts: []int64{1, 1, 0}}
	service := newControlTestService(store, &fakeTelegram{}, true)

	start := time.Now()
	if err := service.waitForSettledJobTypes(context.Background(), []string{domain.JobTypeSendMorningDigest}); err != nil {
		t.Fatalf("waitForSettledJobTypes() error = %v", err)
	}
	if store.activeJobCountCalls != 3 {
		t.Fatalf("active job count calls = %d, want 3", store.activeJobCountCalls)
	}
	if elapsed := time.Since(start); elapsed < 2*controlRunDueSettlePollInterval {
		t.Fatalf("elapsed = %v, want at least %v", elapsed, 2*controlRunDueSettlePollInterval)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
