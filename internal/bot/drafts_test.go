package bot

import (
	"context"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
)

func TestFinishDraftActionSchedulesReliableCleanup(t *testing.T) {
	dashboardID := int64(500)
	draftMessageID := int64(101)
	promptMessageID := int64(102)
	sourceMessageID := int64(103)
	feedbackMessageID := int64(104)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		currentSettings: postgres.UserSettings{
			UserID:             1,
			ChatID:             1,
			Timezone:           "Europe/Moscow",
			DigestLocalTime:    "09:00",
			DashboardMessageID: &dashboardID,
		},
		dashboardStats: domain.DashboardStats{ActiveCount: 1},
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 900}},
	}
	service := newCommandTestService(t, store, tg)

	err := service.finishDraftAction(context.Background(), domain.DraftSession{
		ID:                  10,
		TraceID:             "trace-draft",
		UserID:              1,
		ChatID:              1,
		DraftMessageID:      &draftMessageID,
		EditPromptMessageID: &promptMessageID,
		SourceMessageID:     &sourceMessageID,
		FeedbackMessageID:   &feedbackMessageID,
	}, 1, "ok")
	if err != nil {
		t.Fatalf("finishDraftAction() error = %v", err)
	}
	if len(store.enqueuedJobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(store.enqueuedJobs))
	}
	reliableCleanup := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if reliableCleanup.Origin != "draft_finish" {
		t.Fatalf("reliable cleanup origin = %q, want draft_finish", reliableCleanup.Origin)
	}
	if got := reliableCleanup.MessageIDs; len(got) != 4 || got[0] != 101 || got[1] != 102 || got[2] != 103 || got[3] != 104 {
		t.Fatalf("reliable cleanup message ids = %v, want [101 102 103 104]", got)
	}
	confirmationCleanup := deletePayloadFromJob(t, store.enqueuedJobs[1])
	if confirmationCleanup.Origin != "draft_finish" {
		t.Fatalf("confirmation cleanup origin = %q, want draft_finish", confirmationCleanup.Origin)
	}
	if got := confirmationCleanup.MessageIDs; len(got) != 1 || got[0] != 900 {
		t.Fatalf("confirmation cleanup message ids = %v, want [900]", got)
	}
}

func TestCloseDraftWithStatusDeletesDraftSilently(t *testing.T) {
	dashboardID := int64(500)
	draftMessageID := int64(101)
	promptMessageID := int64(102)
	sourceMessageID := int64(103)
	feedbackMessageID := int64(104)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		currentSettings: postgres.UserSettings{
			UserID:             1,
			ChatID:             1,
			Timezone:           "Europe/Moscow",
			DigestLocalTime:    "09:00",
			DashboardMessageID: &dashboardID,
		},
		dashboardStats: domain.DashboardStats{ActiveCount: 1},
	}
	tg := &fakeTelegram{}
	service := newCommandTestService(t, store, tg)

	err := service.closeDraftWithStatus(context.Background(), 10, domain.DraftSession{
		ID:                  10,
		Status:              domain.DraftStatusReady,
		TraceID:             "trace-draft",
		UserID:              1,
		ChatID:              1,
		DraftMessageID:      &draftMessageID,
		EditPromptMessageID: &promptMessageID,
		SourceMessageID:     &sourceMessageID,
		FeedbackMessageID:   &feedbackMessageID,
	}, 1, domain.DraftStatusCanceled)
	if err != nil {
		t.Fatalf("closeDraftWithStatus() error = %v", err)
	}
	if len(tg.sendRequests) != 0 {
		t.Fatalf("send requests = %d, want 0", len(tg.sendRequests))
	}
	if len(store.enqueuedJobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(store.enqueuedJobs))
	}
	reliableCleanup := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if reliableCleanup.Origin != "draft_finish" {
		t.Fatalf("reliable cleanup origin = %q, want draft_finish", reliableCleanup.Origin)
	}
	if got := reliableCleanup.MessageIDs; len(got) != 4 || got[0] != 101 || got[1] != 102 || got[2] != 103 || got[3] != 104 {
		t.Fatalf("reliable cleanup message ids = %v, want [101 102 103 104]", got)
	}
}
