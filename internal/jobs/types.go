package jobs

import (
	"time"

	"github.com/igor/shelfy/internal/domain"
)

type Envelope struct {
	ID             int64            `json:"id"`
	TraceID        string           `json:"trace_id"`
	JobType        string           `json:"job_type"`
	Status         domain.JobStatus `json:"status"`
	Payload        []byte           `json:"payload"`
	RunAt          time.Time        `json:"run_at"`
	Attempts       int              `json:"attempts"`
	MaxAttempts    int              `json:"max_attempts"`
	IdempotencyKey *string          `json:"idempotency_key,omitempty"`
	LastError      *string          `json:"last_error,omitempty"`
}

type IngestPayload struct {
	TraceID           string             `json:"trace_id"`
	UserID            int64              `json:"user_id"`
	ChatID            int64              `json:"chat_id"`
	MessageID         int64              `json:"message_id"`
	FeedbackMessageID int64              `json:"feedback_message_id,omitempty"`
	FileID            string             `json:"file_id,omitempty"`
	Text              string             `json:"text,omitempty"`
	Kind              domain.MessageKind `json:"kind"`
}

type DeleteMessagesPayload struct {
	TraceID    string  `json:"trace_id"`
	ChatID     int64   `json:"chat_id"`
	MessageIDs []int64 `json:"message_ids"`
}

type MorningDigestPayload struct {
	TraceID string `json:"trace_id"`
	UserID  int64  `json:"user_id"`
	ChatID  int64  `json:"chat_id"`
}
