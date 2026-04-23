package telegram

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequestTimeout(t *testing.T) {
	client := &Client{}

	if got := client.requestTimeout("answerCallbackQuery"); got != 8*time.Second {
		t.Fatalf("answerCallbackQuery timeout = %s, want %s", got, 8*time.Second)
	}

	if got := client.requestTimeout("editMessageText"); got != 20*time.Second {
		t.Fatalf("editMessageText timeout = %s, want %s", got, 20*time.Second)
	}

	if got := client.requestTimeout("sendMessage"); got != 60*time.Second {
		t.Fatalf("sendMessage timeout = %s, want %s", got, 60*time.Second)
	}
	if got := client.requestTimeout("deleteMessages"); got != 20*time.Second {
		t.Fatalf("deleteMessages timeout = %s, want %s", got, 20*time.Second)
	}

	if got := client.requestTimeout("getFile"); got != 30*time.Second {
		t.Fatalf("getFile timeout = %s, want %s", got, 30*time.Second)
	}

	if got := client.requestTimeout("pinChatMessage"); got != 30*time.Second {
		t.Fatalf("pinChatMessage timeout = %s, want %s", got, 30*time.Second)
	}
}

func TestIsTransientPollError(t *testing.T) {
	if !IsTransientPollError(errors.New("Post \"https://api.telegram.org\": local error: tls: bad record MAC")) {
		t.Fatal("expected tls bad record MAC to be treated as transient poll error")
	}
	if !IsTransientPollError(errors.New("write tcp 1.2.3.4:443: write: broken pipe")) {
		t.Fatal("expected broken pipe to be treated as transient poll error")
	}
	if IsTransientPollError(errors.New("telegram getUpdates failed: 401 unauthorized")) {
		t.Fatal("did not expect non-transient API error to be treated as transient")
	}
}

func TestCallJSONRetriesTransientEditMessageError(t *testing.T) {
	attempts := 0
	client := &Client{
		token:  "test-token",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return nil, errors.New("Post \"https://api.telegram.org\": local error: tls: bad record MAC")
				}
				if !strings.Contains(req.URL.String(), "/editMessageText") {
					t.Fatalf("unexpected URL: %s", req.URL.String())
				}
				return jsonResponse(`{"ok":true}`), nil
			}),
		},
	}

	err := client.callJSON(context.Background(), "editMessageText", map[string]any{
		"chat_id":    1,
		"message_id": 2,
		"text":       "ok",
	}, &BaseResponse{})
	if err != nil {
		t.Fatalf("callJSON returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestCallJSONDoesNotRetrySendMessageTransientError(t *testing.T) {
	attempts := 0
	client := &Client{
		token:  "test-token",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				return nil, errors.New("Post \"https://api.telegram.org\": local error: tls: bad record MAC")
			}),
		},
	}

	err := client.callJSON(context.Background(), "sendMessage", map[string]any{
		"chat_id": 1,
		"text":    "ok",
	}, &SendMessageResponse{})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestEditMessageTextReturnsTimeoutError(t *testing.T) {
	client := &Client{
		token:  "test-token",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			}),
		},
	}

	err := client.EditMessageText(context.Background(), EditMessageTextRequest{
		ChatID:    1,
		MessageID: 2,
		Text:      "stats",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestShouldUseFreshConnection(t *testing.T) {
	client := &Client{}

	cases := map[string]bool{
		"sendMessage":         true,
		"editMessageText":     true,
		"deleteMessage":       true,
		"deleteMessages":      true,
		"pinChatMessage":      true,
		"answerCallbackQuery": true,
		"getUpdates":          false,
		"getFile":             false,
	}

	for method, want := range cases {
		if got := client.shouldUseFreshConnection(method); got != want {
			t.Fatalf("%s fresh connection = %v, want %v", method, got, want)
		}
	}
}

func TestIsMissingMessageTargetError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "edit target missing",
			err:  errors.New("telegram editMessageText not ok: Bad Request: message to edit not found"),
			want: true,
		},
		{
			name: "pin target missing",
			err:  errors.New("telegram pinChatMessage not ok: Bad Request: message to pin not found"),
			want: true,
		},
		{
			name: "delete target missing",
			err:  errors.New("telegram deleteMessage not ok: Bad Request: message to delete not found"),
			want: true,
		},
		{
			name: "message id invalid",
			err:  errors.New("telegram pinChatMessage not ok: Bad Request: MESSAGE_ID_INVALID"),
			want: true,
		},
		{
			name: "transport timeout",
			err:  context.DeadlineExceeded,
			want: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMissingMessageTargetError(tt.err); got != tt.want {
				t.Fatalf("IsMissingMessageTargetError() = %v, want %v", got, tt.want)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDeleteMessagesChunksLargeBatches(t *testing.T) {
	requests := 0
	client := &Client{
		token:  "test-token",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if !strings.Contains(req.URL.String(), "/deleteMessages") {
					t.Fatalf("unexpected URL: %s", req.URL.String())
				}
				return jsonResponse(`{"ok":true}`), nil
			}),
		},
	}

	ids := make([]int64, 0, 205)
	for i := 1; i <= 205; i++ {
		ids = append(ids, int64(i))
	}
	if err := client.DeleteMessages(context.Background(), 1, ids); err != nil {
		t.Fatalf("DeleteMessages() error = %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}
