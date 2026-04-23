package bot

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/telegram"
)

type fastPathStub struct {
	handled     bool
	err         error
	called      bool
	lastPayload jobs.IngestPayload
}

func (s *fastPathStub) TryHandleTextFast(_ context.Context, payload jobs.IngestPayload) (bool, error) {
	s.called = true
	s.lastPayload = payload
	return s.handled, s.err
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTryHandleTextFastUsesPayloadWithoutFeedbackMessage(t *testing.T) {
	stub := &fastPathStub{handled: true}
	service := &Service{
		logger:       newDiscardLogger(),
		textFastPath: stub,
	}

	handled, err := service.tryHandleTextFast(context.Background(), jobs.IngestPayload{
		TraceID:   "trace-1",
		UserID:    42,
		ChatID:    42,
		MessageID: 1001,
		Text:      "сметана завтра",
		Kind:      domain.MessageKindText,
	})
	if err != nil {
		t.Fatalf("tryHandleTextFast() error = %v", err)
	}
	if !handled {
		t.Fatal("tryHandleTextFast() handled = false, want true")
	}
	if !stub.called {
		t.Fatal("TryHandleTextFast was not called")
	}
	if stub.lastPayload.FeedbackMessageID != 0 {
		t.Fatalf("FeedbackMessageID = %d, want 0 for fast path", stub.lastPayload.FeedbackMessageID)
	}
}

func TestTryHandleTextFastSkipsNonTextMessages(t *testing.T) {
	stub := &fastPathStub{handled: true}
	service := &Service{
		logger:       newDiscardLogger(),
		textFastPath: stub,
	}

	handled, err := service.tryHandleTextFast(context.Background(), jobs.IngestPayload{
		Kind: domain.MessageKindVoice,
	})
	if err != nil {
		t.Fatalf("tryHandleTextFast() error = %v", err)
	}
	if handled {
		t.Fatal("tryHandleTextFast() handled = true, want false")
	}
	if stub.called {
		t.Fatal("TryHandleTextFast should not be called for non-text messages")
	}
}

func TestHandleInvalidDraftEditMessageSchedulesReliableCleanup(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 700}},
	}
	service := newCommandTestService(t, store, tg)

	err := service.handleInvalidDraftEditMessage(context.Background(), domain.DraftSession{
		ID:      10,
		TraceID: "trace-invalid",
		Status:  domain.DraftStatusEditingDate,
	}, telegram.Message{
		MessageID: 600,
		Chat:      telegram.Chat{ID: 1, Type: "private"},
		Text:      "не дата",
	}, true)
	if err != nil {
		t.Fatalf("handleInvalidDraftEditMessage() error = %v", err)
	}
	if len(store.enqueuedJobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(store.enqueuedJobs))
	}
	userCleanup := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if userCleanup.Origin != "invalid_input" {
		t.Fatalf("user cleanup origin = %q, want invalid_input", userCleanup.Origin)
	}
	if got := userCleanup.MessageIDs; len(got) != 1 || got[0] != 600 {
		t.Fatalf("user cleanup message ids = %v, want [600]", got)
	}
	feedbackCleanup := deletePayloadFromJob(t, store.enqueuedJobs[1])
	if feedbackCleanup.Origin != "invalid_input" {
		t.Fatalf("feedback cleanup origin = %q, want invalid_input", feedbackCleanup.Origin)
	}
	if got := feedbackCleanup.MessageIDs; len(got) != 1 || got[0] != 700 {
		t.Fatalf("feedback cleanup message ids = %v, want [700]", got)
	}
}

func TestHandleUnsupportedMessageSchedulesReliableCleanup(t *testing.T) {
	store := &fakeStore{
		now: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC),
	}
	tg := &fakeTelegram{
		sendResponses: []telegram.Message{{MessageID: 701}},
	}
	service := newCommandTestService(t, store, tg)

	err := service.handleUnsupportedMessage(context.Background(), telegram.Message{
		MessageID: 601,
		Chat:      telegram.Chat{ID: 1, Type: "private"},
		From:      &telegram.User{ID: 1},
	})
	if err != nil {
		t.Fatalf("handleUnsupportedMessage() error = %v", err)
	}
	if len(store.enqueuedJobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(store.enqueuedJobs))
	}
	userCleanup := deletePayloadFromJob(t, store.enqueuedJobs[0])
	if userCleanup.Origin != "unsupported_input" {
		t.Fatalf("user cleanup origin = %q, want unsupported_input", userCleanup.Origin)
	}
	if got := userCleanup.MessageIDs; len(got) != 1 || got[0] != 601 {
		t.Fatalf("user cleanup message ids = %v, want [601]", got)
	}
	feedbackCleanup := deletePayloadFromJob(t, store.enqueuedJobs[1])
	if feedbackCleanup.Origin != "unsupported_input" {
		t.Fatalf("feedback cleanup origin = %q, want unsupported_input", feedbackCleanup.Origin)
	}
	if got := feedbackCleanup.MessageIDs; len(got) != 1 || got[0] != 701 {
		t.Fatalf("feedback cleanup message ids = %v, want [701]", got)
	}
}
