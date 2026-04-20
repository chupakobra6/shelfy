package telegram

import "testing"

func TestIncomingContentType(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		got := incomingContentType(Message{Text: "milk"})
		if got != "text" {
			t.Fatalf("expected text, got %s", got)
		}
	})

	t.Run("photo caption", func(t *testing.T) {
		got := incomingContentType(Message{
			Caption: "milk",
			Photo:   []Photo{{FileID: "1"}},
		})
		if got != "photo_caption" {
			t.Fatalf("expected photo_caption, got %s", got)
		}
	})

	t.Run("voice", func(t *testing.T) {
		got := incomingContentType(Message{Voice: &Voice{FileID: "voice"}})
		if got != "voice" {
			t.Fatalf("expected voice, got %s", got)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		got := incomingContentType(Message{})
		if got != "unknown" {
			t.Fatalf("expected unknown, got %s", got)
		}
	})
}
