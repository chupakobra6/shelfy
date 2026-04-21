package bot

import (
	"testing"

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
