package main

import (
	"testing"

	"github.com/igor/shelfy/internal/telegram"
)

func TestIsTelegramCommand(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		command string
		want    bool
	}{
		{name: "exact command", text: "/start", command: "/start", want: true},
		{name: "command with bot suffix", text: "/dashboard@yaminotoubot", command: "/dashboard", want: true},
		{name: "command with trailing args", text: "/dashboard please", command: "/dashboard", want: true},
		{name: "different command", text: "/start", command: "/dashboard", want: false},
		{name: "plain text", text: "dashboard", command: "/dashboard", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTelegramCommand(tt.text, tt.command); got != tt.want {
				t.Fatalf("isTelegramCommand(%q, %q) = %v, want %v", tt.text, tt.command, got, tt.want)
			}
		})
	}
}

func TestUpdateMailboxKey(t *testing.T) {
	tests := []struct {
		name   string
		update telegram.Update
		want   string
	}{
		{
			name: "message user id",
			update: telegram.Update{
				UpdateID: 1,
				Message: &telegram.Message{
					Chat: telegram.Chat{ID: 10},
					From: &telegram.User{ID: 42},
				},
			},
			want: "42",
		},
		{
			name: "callback user id",
			update: telegram.Update{
				UpdateID: 2,
				CallbackQuery: &telegram.CallbackQuery{
					From: telegram.User{ID: 77},
				},
			},
			want: "77",
		},
		{
			name: "service message falls back to chat",
			update: telegram.Update{
				UpdateID: 3,
				Message: &telegram.Message{
					Chat: telegram.Chat{ID: 99},
				},
			},
			want: "chat:99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateMailboxKey(tt.update); got != tt.want {
				t.Fatalf("updateMailboxKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
