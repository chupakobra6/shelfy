package bot

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	now time.Time

	getUserSettingsQueue []userSettingsResult
	currentSettings      postgres.UserSettings

	upsertedSettings []postgres.UserSettings
	setDashboardIDs  []int64
	enqueuedJobs     []enqueuedJob

	dashboardStats    domain.DashboardStats
	upsertSettingsErr error
	dashboardStatsErr error
	setDashboardErr   error
	enqueueErr        error

	listPageResults        []listPageResult
	listPageCalls          []listPageCall
	product                domain.Product
	getProductErr          error
	updatedProductStatuses []updatedProductStatus
}

type userSettingsResult struct {
	settings postgres.UserSettings
	err      error
}

type enqueuedJob struct {
	traceID        string
	jobType        string
	payload        any
	runAt          time.Time
	idempotencyKey *string
}

type listPageResult struct {
	products   []domain.Product
	totalCount int
	err        error
}

type listPageCall struct {
	mode   string
	limit  int
	offset int
}

type updatedProductStatus struct {
	productID int64
	status    domain.ProductStatus
}

func (s *fakeStore) CurrentNow(_ context.Context, fallback time.Time) (time.Time, error) {
	if !s.now.IsZero() {
		return s.now, nil
	}
	return fallback, nil
}

func (s *fakeStore) EnqueueJob(_ context.Context, traceID, jobType string, payload any, runAt time.Time, idempotencyKey *string) error {
	if s.enqueueErr != nil {
		return s.enqueueErr
	}
	s.enqueuedJobs = append(s.enqueuedJobs, enqueuedJob{
		traceID:        traceID,
		jobType:        jobType,
		payload:        payload,
		runAt:          runAt,
		idempotencyKey: idempotencyKey,
	})
	return nil
}

func (s *fakeStore) FindEditableDraft(context.Context, int64) (domain.DraftSession, bool, error) {
	return domain.DraftSession{}, false, nil
}

func (s *fakeStore) GetUserSettings(_ context.Context, _ int64) (postgres.UserSettings, error) {
	if len(s.getUserSettingsQueue) > 0 {
		item := s.getUserSettingsQueue[0]
		s.getUserSettingsQueue = s.getUserSettingsQueue[1:]
		if item.err == nil {
			s.currentSettings = item.settings
		}
		return item.settings, item.err
	}
	return s.currentSettings, nil
}

func (s *fakeStore) UpsertUserSettings(_ context.Context, settings postgres.UserSettings) error {
	if s.upsertSettingsErr != nil {
		return s.upsertSettingsErr
	}
	s.upsertedSettings = append(s.upsertedSettings, settings)
	s.currentSettings = settings
	return nil
}

func (s *fakeStore) DashboardStats(context.Context, int64, time.Time) (domain.DashboardStats, error) {
	if s.dashboardStatsErr != nil {
		return domain.DashboardStats{}, s.dashboardStatsErr
	}
	return s.dashboardStats, nil
}

func (s *fakeStore) SetDashboardMessageID(_ context.Context, _ int64, messageID int64) error {
	if s.setDashboardErr != nil {
		return s.setDashboardErr
	}
	s.setDashboardIDs = append(s.setDashboardIDs, messageID)
	s.currentSettings.DashboardMessageID = &messageID
	return nil
}

func (s *fakeStore) ListVisibleProducts(context.Context, int64, string, time.Time) ([]domain.Product, error) {
	return nil, nil
}

func (s *fakeStore) ListVisibleProductsPage(_ context.Context, _ int64, mode string, _ time.Time, limit, offset int) ([]domain.Product, int, error) {
	s.listPageCalls = append(s.listPageCalls, listPageCall{
		mode:   mode,
		limit:  limit,
		offset: offset,
	})
	if len(s.listPageResults) > 0 {
		result := s.listPageResults[0]
		s.listPageResults = s.listPageResults[1:]
		return result.products, result.totalCount, result.err
	}
	return nil, 0, nil
}

func (s *fakeStore) CreateProductFromDraft(context.Context, int64) (domain.Product, error) {
	return domain.Product{}, nil
}

func (s *fakeStore) GetDraftSession(context.Context, int64) (domain.DraftSession, error) {
	return domain.DraftSession{}, nil
}

func (s *fakeStore) UpdateDraftStatus(context.Context, int64, domain.DraftStatus) error {
	return nil
}

func (s *fakeStore) GetProduct(context.Context, int64) (domain.Product, error) {
	if s.getProductErr != nil {
		return domain.Product{}, s.getProductErr
	}
	return s.product, nil
}

func (s *fakeStore) UpdateProductStatus(_ context.Context, productID int64, status domain.ProductStatus) error {
	s.updatedProductStatuses = append(s.updatedProductStatuses, updatedProductStatus{
		productID: productID,
		status:    status,
	})
	return nil
}

func (s *fakeStore) UpdateUserDigestLocalTime(context.Context, int64, string) error {
	return nil
}

func (s *fakeStore) UpdateUserTimezone(context.Context, int64, string) error {
	return nil
}

func (s *fakeStore) SetDraftEditPromptMessageID(context.Context, int64, *int64) error {
	return nil
}

func (s *fakeStore) SaveIngestEvent(context.Context, string, int64, int64, int64, domain.MessageKind, string, string, map[string]any) error {
	return nil
}

func (s *fakeStore) UpdateDraftFields(context.Context, int64, string, *time.Time, string, domain.DraftStatus) error {
	return nil
}

type fakeTelegram struct {
	sendRequests   []telegram.SendMessageRequest
	editRequests   []telegram.EditMessageTextRequest
	deleteRequests []int64
	pinRequests    []int64

	sendResponses []telegram.Message
	sendErrs      []error
	editErrs      []error
	pinErrs       []error
	deleteErrs    []error
}

func (t *fakeTelegram) SendMessage(_ context.Context, request telegram.SendMessageRequest) (telegram.Message, error) {
	t.sendRequests = append(t.sendRequests, request)
	if len(t.sendErrs) > 0 {
		err := t.sendErrs[0]
		t.sendErrs = t.sendErrs[1:]
		if err != nil {
			return telegram.Message{}, err
		}
	}
	if len(t.sendResponses) > 0 {
		response := t.sendResponses[0]
		t.sendResponses = t.sendResponses[1:]
		return response, nil
	}
	return telegram.Message{MessageID: int64(1000 + len(t.sendRequests))}, nil
}

func (t *fakeTelegram) EditMessageText(_ context.Context, request telegram.EditMessageTextRequest) error {
	t.editRequests = append(t.editRequests, request)
	if len(t.editErrs) > 0 {
		err := t.editErrs[0]
		t.editErrs = t.editErrs[1:]
		return err
	}
	return nil
}

func (t *fakeTelegram) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	t.deleteRequests = append(t.deleteRequests, messageID)
	if len(t.deleteErrs) > 0 {
		err := t.deleteErrs[0]
		t.deleteErrs = t.deleteErrs[1:]
		return err
	}
	return nil
}

func (t *fakeTelegram) PinMessage(_ context.Context, _ int64, messageID int64) error {
	t.pinRequests = append(t.pinRequests, messageID)
	if len(t.pinErrs) > 0 {
		err := t.pinErrs[0]
		t.pinErrs = t.pinErrs[1:]
		return err
	}
	return nil
}

func (t *fakeTelegram) AnswerCallbackQuery(context.Context, telegram.AnswerCallbackQueryRequest) error {
	return nil
}

func newCommandTestService(t *testing.T, store *fakeStore, tg *fakeTelegram) *Service {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	loader, err := copycat.Load(filepath.Join(filepath.Dir(filename), "..", "..", "assets", "copy", "runtime.ru.yaml"))
	if err != nil {
		t.Fatalf("load runtime copy: %v", err)
	}
	return NewService(store, tg, loader, newDiscardLogger(), "Europe/Moscow", "09:00", nil)
}

func deletePayloadFromJob(t *testing.T, job enqueuedJob) jobs.DeleteMessagesPayload {
	t.Helper()
	payload, ok := job.payload.(jobs.DeleteMessagesPayload)
	if !ok {
		t.Fatalf("payload type = %T, want jobs.DeleteMessagesPayload", job.payload)
	}
	return payload
}

func TestEnsureUserSettingsPreservesExistingPreferences(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:             1,
				ChatID:             100,
				Timezone:           "Europe/Berlin",
				DigestLocalTime:    "08:00",
				DashboardMessageID: &dashboardID,
			},
		}},
	}
	service := newCommandTestService(t, store, &fakeTelegram{})

	settings, err := service.ensureUserSettings(context.Background(), 1, 200)
	if err != nil {
		t.Fatalf("ensureUserSettings() error = %v", err)
	}
	if settings.ChatID != 200 {
		t.Fatalf("ChatID = %d, want 200", settings.ChatID)
	}
	if settings.Timezone != "Europe/Berlin" {
		t.Fatalf("Timezone = %q, want Europe/Berlin", settings.Timezone)
	}
	if settings.DigestLocalTime != "08:00" {
		t.Fatalf("DigestLocalTime = %q, want 08:00", settings.DigestLocalTime)
	}
	if len(store.upsertedSettings) != 1 {
		t.Fatalf("upserted settings count = %d, want 1", len(store.upsertedSettings))
	}
	if store.upsertedSettings[0].Timezone != "Europe/Berlin" {
		t.Fatalf("upserted timezone = %q, want Europe/Berlin", store.upsertedSettings[0].Timezone)
	}
}

func TestHandleStartBootstrapCreatesDashboard(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			err: pgx.ErrNoRows,
		}},
		dashboardStats: domain.DashboardStats{ActiveCount: 3, SoonCount: 1, ExpiredCount: 0},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 501}},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleStart(context.Background(), 1, 1, 900); err != nil {
		t.Fatalf("HandleStart() error = %v", err)
	}
	if len(tg.sendRequests) != 1 {
		t.Fatalf("send requests = %d, want 1", len(tg.sendRequests))
	}
	if tg.sendRequests[0].ReplyMarkup == nil {
		t.Fatal("expected dashboard send to include reply markup")
	}
	if len(store.setDashboardIDs) != 1 || store.setDashboardIDs[0] != 501 {
		t.Fatalf("set dashboard ids = %v, want [501]", store.setDashboardIDs)
	}
	if len(tg.pinRequests) != 1 || tg.pinRequests[0] != 501 {
		t.Fatalf("pin requests = %v, want [501]", tg.pinRequests)
	}
}

func TestHandleStartExistingDashboardOnlyRepins(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:             1,
				ChatID:             1,
				Timezone:           "Europe/Moscow",
				DigestLocalTime:    "09:00",
				DashboardMessageID: &dashboardID,
			},
		}},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleStart(context.Background(), 1, 1, 901); err != nil {
		t.Fatalf("HandleStart() error = %v", err)
	}
	if len(tg.sendRequests) != 0 {
		t.Fatalf("send requests = %d, want 0", len(tg.sendRequests))
	}
	if len(store.setDashboardIDs) != 0 {
		t.Fatalf("set dashboard ids = %v, want none", store.setDashboardIDs)
	}
	if len(tg.pinRequests) != 1 || tg.pinRequests[0] != 42 {
		t.Fatalf("pin requests = %v, want [42]", tg.pinRequests)
	}
}

func TestHandleStartBrokenDashboardPromptsRecovery(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:             1,
				ChatID:             1,
				Timezone:           "Europe/Moscow",
				DigestLocalTime:    "09:00",
				DashboardMessageID: &dashboardID,
			},
		}},
	}
	tg := &fakeTelegram{
		pinErrs:       []error{errors.New("telegram pinChatMessage not ok: Bad Request: message to pin not found")},
		sendResponses: []telegram.Message{{MessageID: 777}},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleStart(context.Background(), 1, 1, 902); err != nil {
		t.Fatalf("HandleStart() error = %v", err)
	}
	if len(tg.pinRequests) != 1 || tg.pinRequests[0] != 42 {
		t.Fatalf("pin requests = %v, want [42]", tg.pinRequests)
	}
	if len(tg.sendRequests) != 1 {
		t.Fatalf("send requests = %d, want 1", len(tg.sendRequests))
	}
	if tg.sendRequests[0].ReplyMarkup != nil {
		t.Fatal("expected recovery hint without reply markup")
	}
	if len(store.enqueuedJobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(store.enqueuedJobs))
	}
}

func TestHandleDashboardCommandRecreatesExistingDashboard(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:             1,
				ChatID:             1,
				Timezone:           "Europe/Moscow",
				DigestLocalTime:    "09:00",
				DashboardMessageID: &dashboardID,
			},
		}},
		dashboardStats: domain.DashboardStats{ActiveCount: 2, SoonCount: 1, ExpiredCount: 0},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 602}},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleDashboardCommand(context.Background(), 1, 1, 903); err != nil {
		t.Fatalf("HandleDashboardCommand() error = %v", err)
	}
	if len(tg.sendRequests) != 1 {
		t.Fatalf("send requests = %d, want 1", len(tg.sendRequests))
	}
	if len(tg.editRequests) != 0 {
		t.Fatalf("edit requests = %+v, want none", tg.editRequests)
	}
	if len(store.setDashboardIDs) != 1 || store.setDashboardIDs[0] != 602 {
		t.Fatalf("set dashboard ids = %v, want [602]", store.setDashboardIDs)
	}
	if len(tg.pinRequests) != 1 || tg.pinRequests[0] != 602 {
		t.Fatalf("pin requests = %v, want [602]", tg.pinRequests)
	}
	if len(store.enqueuedJobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(store.enqueuedJobs))
	}
	replacedCleanup := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if replacedCleanup.Origin != "dashboard_replaced" {
		t.Fatalf("cleanup origin = %q, want dashboard_replaced", replacedCleanup.Origin)
	}
	if got := replacedCleanup.MessageIDs; len(got) != 1 || got[0] != 42 {
		t.Fatalf("cleanup message ids = %v, want [42]", got)
	}
}

func TestHandleDashboardCommandCreatesWhenMissing(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:          1,
				ChatID:          1,
				Timezone:        "Europe/Moscow",
				DigestLocalTime: "09:00",
			},
		}},
		dashboardStats: domain.DashboardStats{ActiveCount: 1, SoonCount: 0, ExpiredCount: 0},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 601}},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleDashboardCommand(context.Background(), 1, 1, 904); err != nil {
		t.Fatalf("HandleDashboardCommand() error = %v", err)
	}
	if len(tg.sendRequests) != 1 {
		t.Fatalf("send requests = %d, want 1", len(tg.sendRequests))
	}
	if len(store.setDashboardIDs) != 1 || store.setDashboardIDs[0] != 601 {
		t.Fatalf("set dashboard ids = %v, want [601]", store.setDashboardIDs)
	}
	if len(tg.pinRequests) != 1 || tg.pinRequests[0] != 601 {
		t.Fatalf("pin requests = %v, want [601]", tg.pinRequests)
	}
}

func TestHandleStartSchedulesFallbackCleanupWhenImmediateDeleteFails(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			err: pgx.ErrNoRows,
		}},
		dashboardStats: domain.DashboardStats{ActiveCount: 1},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 801}},
		deleteErrs:    []error{errors.New("write tcp timeout")},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleStart(context.Background(), 1, 1, 990); err != nil {
		t.Fatalf("HandleStart() error = %v", err)
	}
	if len(store.enqueuedJobs) == 0 {
		t.Fatal("expected cleanup job to be scheduled")
	}
	payload := deletePayloadFromJob(t, store.enqueuedJobs[len(store.enqueuedJobs)-1])
	if payload.Origin != "start_command" {
		t.Fatalf("payload origin = %q, want start_command", payload.Origin)
	}
	if len(payload.MessageIDs) != 1 || payload.MessageIDs[0] != 990 {
		t.Fatalf("payload message ids = %v, want [990]", payload.MessageIDs)
	}
}

func TestHandleDashboardCommandSchedulesFallbackCleanupWhenImmediateDeleteFails(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{
			{settings: postgres.UserSettings{UserID: 1, ChatID: 1, Timezone: "Europe/Moscow", DigestLocalTime: "09:00", DashboardMessageID: &dashboardID}},
			{settings: postgres.UserSettings{UserID: 1, ChatID: 1, Timezone: "Europe/Moscow", DigestLocalTime: "09:00", DashboardMessageID: &dashboardID}},
		},
		dashboardStats: domain.DashboardStats{ActiveCount: 2},
	}
	tg := &fakeTelegram{
		deleteErrs: []error{errors.New("write tcp timeout")},
	}
	service := newCommandTestService(t, store, tg)

	if err := service.HandleDashboardCommand(context.Background(), 1, 1, 991); err != nil {
		t.Fatalf("HandleDashboardCommand() error = %v", err)
	}
	if len(store.enqueuedJobs) == 0 {
		t.Fatal("expected cleanup job to be scheduled")
	}
	payload := deletePayloadFromJob(t, store.enqueuedJobs[len(store.enqueuedJobs)-1])
	if payload.Origin != "dashboard_command" {
		t.Fatalf("payload origin = %q, want dashboard_command", payload.Origin)
	}
	if len(payload.MessageIDs) != 1 || payload.MessageIDs[0] != 991 {
		t.Fatalf("payload message ids = %v, want [991]", payload.MessageIDs)
	}
}

func TestEnsureCurrentDashboardCallbackRejectsStaleMessage(t *testing.T) {
	dashboardID := int64(42)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		getUserSettingsQueue: []userSettingsResult{{
			settings: postgres.UserSettings{
				UserID:             1,
				ChatID:             1,
				Timezone:           "Europe/Moscow",
				DigestLocalTime:    "09:00",
				DashboardMessageID: &dashboardID,
			},
		}},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 900}},
	}
	service := newCommandTestService(t, store, tg)

	current, err := service.ensureCurrentDashboardCallback(context.Background(), 1, 41, 1)
	if err != nil {
		t.Fatalf("ensureCurrentDashboardCallback() error = %v", err)
	}
	if current {
		t.Fatal("expected stale dashboard callback to be rejected")
	}
	if len(tg.sendRequests) != 1 {
		t.Fatalf("send requests = %d, want 1", len(tg.sendRequests))
	}
	if len(store.enqueuedJobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(store.enqueuedJobs))
	}
}

func TestScheduleDeleteMessagesJobTypeForRecoveryPrompt(t *testing.T) {
	store := &fakeStore{now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)}
	tg := &fakeTelegram{sendResponses: []telegram.Message{{MessageID: 800}}}
	service := newCommandTestService(t, store, tg)

	if err := service.promptDashboardRecovery(context.Background(), 1); err != nil {
		t.Fatalf("promptDashboardRecovery() error = %v", err)
	}
	if len(store.enqueuedJobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(store.enqueuedJobs))
	}
	if store.enqueuedJobs[0].jobType != domain.JobTypeDeleteMessages {
		t.Fatalf("jobType = %q, want %q", store.enqueuedJobs[0].jobType, domain.JobTypeDeleteMessages)
	}
	payload, ok := store.enqueuedJobs[0].payload.(jobs.DeleteMessagesPayload)
	if !ok {
		t.Fatalf("payload type = %T, want jobs.DeleteMessagesPayload", store.enqueuedJobs[0].payload)
	}
	if len(payload.MessageIDs) != 1 || payload.MessageIDs[0] != 800 {
		t.Fatalf("payload message ids = %v, want [800]", payload.MessageIDs)
	}
}

func TestHandleDashboardCallbackUsesExplicitPage(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		listPageResults: []listPageResult{{
			products:   []domain.Product{{ID: 1, Name: "кефир", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)}},
			totalCount: 9,
		}},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	callback := telegram.CallbackQuery{
		From: telegram.User{ID: 1},
		Message: &telegram.Message{
			MessageID: 42,
			Chat:      telegram.Chat{ID: 99},
		},
	}
	if err := service.handleDashboardCallback(context.Background(), callback, []string{"dashboard", "list", "page", "1"}); err != nil {
		t.Fatalf("handleDashboardCallback() error = %v", err)
	}
	if len(store.listPageCalls) != 1 {
		t.Fatalf("list page calls = %d, want 1", len(store.listPageCalls))
	}
	if got := store.listPageCalls[0]; got.mode != "active" || got.limit != dashboardListPageSize || got.offset != dashboardListPageSize {
		t.Fatalf("unexpected paging call: %+v", got)
	}
	if len(tg.editRequests) != 1 {
		t.Fatalf("edit requests = %d, want 1", len(tg.editRequests))
	}
	if got := tg.editRequests[0].ReplyMarkup; got == nil || len(got.InlineKeyboard) == 0 || got.InlineKeyboard[len(got.InlineKeyboard)-1][0].CallbackData != "dashboard:home" {
		t.Fatalf("unexpected list markup: %+v", got)
	}
}

func TestHandleDashboardCallbackClampsOutOfRangePage(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		listPageResults: []listPageResult{
			{products: nil, totalCount: 8},
			{products: []domain.Product{{ID: 1, Name: "кефир", ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)}}, totalCount: 8},
		},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	callback := telegram.CallbackQuery{
		From: telegram.User{ID: 1},
		Message: &telegram.Message{
			MessageID: 42,
			Chat:      telegram.Chat{ID: 99},
		},
	}
	if err := service.handleDashboardCallback(context.Background(), callback, []string{"dashboard", "list", "page", "5"}); err != nil {
		t.Fatalf("handleDashboardCallback() error = %v", err)
	}
	if len(store.listPageCalls) != 2 {
		t.Fatalf("list page calls = %d, want 2", len(store.listPageCalls))
	}
	if store.listPageCalls[0].offset != 5*dashboardListPageSize {
		t.Fatalf("first offset = %d, want %d", store.listPageCalls[0].offset, 5*dashboardListPageSize)
	}
	if store.listPageCalls[1].offset != 0 {
		t.Fatalf("second offset = %d, want 0", store.listPageCalls[1].offset)
	}
}

func TestHandleProductCallbackOpenPreservesOrigin(t *testing.T) {
	store := &fakeStore{
		product: domain.Product{
			ID:        77,
			Name:      "сметана",
			ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
			Status:    domain.ProductStatusActive,
		},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	callback := telegram.CallbackQuery{
		From: telegram.User{ID: 1},
		Message: &telegram.Message{
			MessageID: 42,
			Chat:      telegram.Chat{ID: 99},
		},
	}
	if err := service.handleProductCallback(context.Background(), callback, []string{"product", "open", "77", "soon", "1"}); err != nil {
		t.Fatalf("handleProductCallback() error = %v", err)
	}
	if len(tg.editRequests) != 1 {
		t.Fatalf("edit requests = %d, want 1", len(tg.editRequests))
	}
	markup := tg.editRequests[0].ReplyMarkup
	if markup == nil {
		t.Fatal("expected reply markup")
	}
	if got := markup.InlineKeyboard[2][0].CallbackData; got != "dashboard:soon:page:1" {
		t.Fatalf("back callback = %q, want dashboard:soon:page:1", got)
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != "product:set:77:consumed:soon:1" {
		t.Fatalf("status callback = %q, want product:set:77:consumed:soon:1", got)
	}
}

func TestHandleProductCallbackOpenLegacyFallsBackToListPageZero(t *testing.T) {
	store := &fakeStore{
		product: domain.Product{
			ID:        77,
			Name:      "сметана",
			ExpiresOn: time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
			Status:    domain.ProductStatusActive,
		},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	callback := telegram.CallbackQuery{
		From: telegram.User{ID: 1},
		Message: &telegram.Message{
			MessageID: 42,
			Chat:      telegram.Chat{ID: 99},
		},
	}
	if err := service.handleProductCallback(context.Background(), callback, []string{"product", "open", "77"}); err != nil {
		t.Fatalf("handleProductCallback() error = %v", err)
	}
	markup := tg.editRequests[0].ReplyMarkup
	if got := markup.InlineKeyboard[2][0].CallbackData; got != "dashboard:list" {
		t.Fatalf("back callback = %q, want dashboard:list", got)
	}
}

func TestHandleProductCallbackStatusReturnsToOriginPageAndClamps(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		listPageResults: []listPageResult{
			{products: nil, totalCount: 8},
			{products: []domain.Product{{ID: 2, Name: "кефир", ExpiresOn: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)}}, totalCount: 8},
		},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	callback := telegram.CallbackQuery{
		From: telegram.User{ID: 1},
		Message: &telegram.Message{
			MessageID: 42,
			Chat:      telegram.Chat{ID: 99},
		},
	}
	if err := service.handleProductCallback(context.Background(), callback, []string{"product", "set", "77", "consumed", "soon", "1"}); err != nil {
		t.Fatalf("handleProductCallback() error = %v", err)
	}
	if len(store.updatedProductStatuses) != 1 {
		t.Fatalf("updated statuses = %d, want 1", len(store.updatedProductStatuses))
	}
	if got := store.updatedProductStatuses[0]; got.productID != 77 || got.status != domain.ProductStatusConsumed {
		t.Fatalf("unexpected updated status: %+v", got)
	}
	if len(store.listPageCalls) != 2 {
		t.Fatalf("list page calls = %d, want 2", len(store.listPageCalls))
	}
	if store.listPageCalls[0].mode != "soon" || store.listPageCalls[0].offset != dashboardListPageSize {
		t.Fatalf("unexpected first page call: %+v", store.listPageCalls[0])
	}
	if store.listPageCalls[1].offset != 0 {
		t.Fatalf("unexpected clamped offset: %+v", store.listPageCalls[1])
	}
	if len(tg.editRequests) != 1 {
		t.Fatalf("edit requests = %d, want 1", len(tg.editRequests))
	}
	if got := tg.editRequests[0].ReplyMarkup; got == nil || got.InlineKeyboard[len(got.InlineKeyboard)-1][0].CallbackData != "dashboard:home" {
		t.Fatalf("unexpected soon markup: %+v", got)
	}
}
