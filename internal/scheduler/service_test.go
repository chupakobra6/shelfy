package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/jobs"
	"github.com/igor/shelfy/internal/telegram"
)

type fakeTelegram struct {
	deleteErrs     []error
	deleteBatchErr error
	deleteRequests []int64
	deleteBatches  [][]int64
	sendResponses  []telegram.Message
	sendRequests   []telegram.SendMessageRequest
}

func (t *fakeTelegram) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	t.deleteRequests = append(t.deleteRequests, messageID)
	if len(t.deleteErrs) == 0 {
		return nil
	}
	err := t.deleteErrs[0]
	t.deleteErrs = t.deleteErrs[1:]
	return err
}

func (t *fakeTelegram) DeleteMessages(_ context.Context, _ int64, messageIDs []int64) error {
	copied := append([]int64(nil), messageIDs...)
	t.deleteBatches = append(t.deleteBatches, copied)
	return t.deleteBatchErr
}

func (t *fakeTelegram) SendMessage(_ context.Context, request telegram.SendMessageRequest) (telegram.Message, error) {
	t.sendRequests = append(t.sendRequests, request)
	if len(t.sendResponses) > 0 {
		response := t.sendResponses[0]
		t.sendResponses = t.sendResponses[1:]
		return response, nil
	}
	return telegram.Message{MessageID: 1000}, nil
}

func newTestService(tg TelegramAPI) *Service {
	return &Service{
		tg:     tg,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestProcessJobDeleteMessagesIgnoresMissingTarget(t *testing.T) {
	tg := &fakeTelegram{
		deleteErrs: []error{errors.New("telegram deleteMessage not ok: Bad Request: message to delete not found")},
	}
	service := newTestService(tg)
	payload, err := json.Marshal(jobs.DeleteMessagesPayload{
		TraceID:    "trace-1",
		Origin:     "start_command",
		ChatID:     1,
		MessageIDs: []int64{101},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	err = service.ProcessJob(context.Background(), jobs.Envelope{
		TraceID: "trace-1",
		JobType: domain.JobTypeDeleteMessages,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("ProcessJob() error = %v, want nil", err)
	}
}

func TestProcessJobDeleteMessagesReturnsRetryableError(t *testing.T) {
	tg := &fakeTelegram{
		deleteErrs: []error{errors.New("write tcp timeout")},
	}
	service := newTestService(tg)
	payload, err := json.Marshal(jobs.DeleteMessagesPayload{
		TraceID:    "trace-2",
		Origin:     "pin_service",
		ChatID:     1,
		MessageIDs: []int64{102},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	err = service.ProcessJob(context.Background(), jobs.Envelope{
		TraceID: "trace-2",
		JobType: domain.JobTypeDeleteMessages,
		Payload: payload,
	})
	if err == nil {
		t.Fatal("ProcessJob() error = nil, want retryable error")
	}
}

func TestProcessJobDeleteMessagesMixedBatchRetries(t *testing.T) {
	tg := &fakeTelegram{
		deleteBatchErr: errors.New("write tcp timeout"),
		deleteErrs: []error{
			errors.New("telegram deleteMessage not ok: Bad Request: message to delete not found"),
			errors.New("write tcp timeout"),
		},
	}
	service := newTestService(tg)
	payload, err := json.Marshal(jobs.DeleteMessagesPayload{
		TraceID:    "trace-3",
		Origin:     "draft_finish",
		ChatID:     1,
		MessageIDs: []int64{103, 104},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	err = service.ProcessJob(context.Background(), jobs.Envelope{
		TraceID: "trace-3",
		JobType: domain.JobTypeDeleteMessages,
		Payload: payload,
	})
	if err == nil {
		t.Fatal("ProcessJob() error = nil, want retryable error")
	}
	if len(tg.deleteRequests) != 2 {
		t.Fatalf("delete requests = %v, want 2 attempts", tg.deleteRequests)
	}
}

func TestProcessJobDeleteMessagesUsesBatchWhenAvailable(t *testing.T) {
	tg := &fakeTelegram{}
	service := newTestService(tg)
	payload, err := json.Marshal(jobs.DeleteMessagesPayload{
		TraceID:    "trace-4",
		Origin:     "draft_finish",
		ChatID:     1,
		MessageIDs: []int64{201, 202},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	err = service.ProcessJob(context.Background(), jobs.Envelope{
		TraceID: "trace-4",
		JobType: domain.JobTypeDeleteMessages,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("ProcessJob() error = %v", err)
	}
	if len(tg.deleteBatches) != 1 {
		t.Fatalf("delete batches = %v, want 1 batch", tg.deleteBatches)
	}
	if len(tg.deleteRequests) != 0 {
		t.Fatalf("delete requests = %v, want no per-message fallback", tg.deleteRequests)
	}
}

func TestShouldMarkDigestDeletedAfterCleanup(t *testing.T) {
	if !shouldMarkDigestDeletedAfterCleanup(nil) {
		t.Fatal("nil error should mark digest deleted")
	}
	if !shouldMarkDigestDeletedAfterCleanup(errors.New("telegram deleteMessage not ok: Bad Request: message to delete not found")) {
		t.Fatal("missing delete target should mark digest deleted")
	}
	if shouldMarkDigestDeletedAfterCleanup(errors.New("write tcp timeout")) {
		t.Fatal("transient delete failure should defer digest cleanup")
	}
}
