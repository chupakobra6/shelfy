package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/storage/postgres"
	"github.com/igor/shelfy/internal/telegram"
)

func TestShouldDeletePinnedDashboardServiceMessage(t *testing.T) {
	currentDashboardMessageID := int64(100)

	t.Run("matches current private dashboard pin", func(t *testing.T) {
		got := shouldDeletePinnedDashboardServiceMessage(&currentDashboardMessageID, telegram.Message{
			Chat:          telegram.Chat{Type: "private"},
			PinnedMessage: &telegram.MessageRef{MessageID: 100},
		})
		if !got {
			t.Fatal("expected pinned dashboard service message to be deleted")
		}
	})

	t.Run("ignores different dashboard", func(t *testing.T) {
		got := shouldDeletePinnedDashboardServiceMessage(&currentDashboardMessageID, telegram.Message{
			Chat:          telegram.Chat{Type: "private"},
			PinnedMessage: &telegram.MessageRef{MessageID: 101},
		})
		if got {
			t.Fatal("expected unrelated pinned service message to be ignored")
		}
	})

	t.Run("ignores non private chats", func(t *testing.T) {
		got := shouldDeletePinnedDashboardServiceMessage(&currentDashboardMessageID, telegram.Message{
			Chat:          telegram.Chat{Type: "group"},
			PinnedMessage: &telegram.MessageRef{MessageID: 100},
		})
		if got {
			t.Fatal("expected non-private pinned service message to be ignored")
		}
	})

	t.Run("ignores missing pinned payload", func(t *testing.T) {
		got := shouldDeletePinnedDashboardServiceMessage(&currentDashboardMessageID, telegram.Message{
			Chat: telegram.Chat{Type: "private"},
		})
		if got {
			t.Fatal("expected message without pinned payload to be ignored")
		}
	})

	t.Run("ignores missing dashboard", func(t *testing.T) {
		got := shouldDeletePinnedDashboardServiceMessage(nil, telegram.Message{
			Chat:          telegram.Chat{Type: "private"},
			PinnedMessage: &telegram.MessageRef{MessageID: 100},
		})
		if got {
			t.Fatal("expected message without current dashboard to be ignored")
		}
	})
}

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
