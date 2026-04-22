package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
)

func TestHandleServiceMessageSchedulesFallbackCleanupWhenImmediateDeleteFails(t *testing.T) {
	dashboardID := int64(100)
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
		currentSettings: postgres.UserSettings{
			UserID:             1,
			ChatID:             1,
			Timezone:           "Europe/Moscow",
			DigestLocalTime:    "09:00",
			DashboardMessageID: &dashboardID,
		},
	}
	tg := &fakeTelegram{
		deleteErrs: []error{errors.New("write tcp timeout")},
	}
	service := newCommandTestService(t, store, tg)

	handled, err := service.handleServiceMessage(context.Background(), telegram.Message{
		MessageID:     555,
		Chat:          telegram.Chat{ID: 1, Type: "private"},
		PinnedMessage: &telegram.MessageRef{MessageID: 100},
	})
	if err != nil {
		t.Fatalf("handleServiceMessage() error = %v", err)
	}
	if !handled {
		t.Fatal("expected pinned service message to be handled")
	}
	if len(store.enqueuedJobs) != 1 {
		t.Fatalf("enqueued jobs = %d, want 1", len(store.enqueuedJobs))
	}
	payload := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if payload.Origin != "pin_service" {
		t.Fatalf("payload origin = %q, want pin_service", payload.Origin)
	}
	if len(payload.MessageIDs) != 1 || payload.MessageIDs[0] != 555 {
		t.Fatalf("payload message ids = %v, want [555]", payload.MessageIDs)
	}
}
