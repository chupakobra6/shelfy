package domain

import "time"

type MessageKind string

const (
	MessageKindText        MessageKind = "text"
	MessageKindVoice       MessageKind = "voice"
	MessageKindAudio       MessageKind = "audio"
	MessageKindUnsupported MessageKind = "unsupported"
)

type ProductStatus string

const (
	ProductStatusActive    ProductStatus = "active"
	ProductStatusConsumed  ProductStatus = "consumed"
	ProductStatusDiscarded ProductStatus = "discarded"
	ProductStatusDeleted   ProductStatus = "deleted"
)

type DraftStatus string

const (
	DraftStatusPendingInput DraftStatus = "pending_input"
	DraftStatusReady        DraftStatus = "ready"
	DraftStatusEditingName  DraftStatus = "editing_name"
	DraftStatusEditingDate  DraftStatus = "editing_date"
	DraftStatusConfirmed    DraftStatus = "confirmed"
	DraftStatusCanceled     DraftStatus = "canceled"
	DraftStatusDeleted      DraftStatus = "deleted"
	DraftStatusFailed       DraftStatus = "failed"
)

type JobStatus string

const (
	JobStatusQueued  JobStatus = "queued"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
	JobStatusRetry   JobStatus = "retry"
)

const (
	JobTypeIngestText        = "ingest_text"
	JobTypeIngestAudio       = "ingest_audio"
	JobTypeCleanDraft        = "clean_draft"
	JobTypeDeleteMessages    = "delete_messages"
	JobTypeSendMorningDigest = "send_morning_digest"
)

const (
	DraftPayloadKeyNormalizedInput = "normalized_input"
	DraftPayloadKeyCleanerCalled   = "cleaner_called"
	DraftPayloadKeyCleanedInput    = "cleaned_input"
	DraftPayloadKeySelectionReason = "selection_reason"
	DraftPayloadKeyChosenSource    = "chosen_source"
	DraftPayloadKeyCleanerPending  = "cleaner_pending"
)

type Product struct {
	ID                int64
	UserID            int64
	Name              string
	NormalizedName    string
	ExpiresOn         time.Time
	RawDeadlinePhrase string
	Status            ProductStatus
	SourceKind        MessageKind
	CreatedAt         time.Time
	ClosedAt          *time.Time
}

type DraftSession struct {
	ID                  int64
	TraceID             string
	UserID              int64
	ChatID              int64
	SourceKind          MessageKind
	Status              DraftStatus
	SourceMessageID     *int64
	DraftMessageID      *int64
	FeedbackMessageID   *int64
	EditPromptMessageID *int64
	ConfirmedProductID  *int64
	DraftName           string
	DraftExpiresOn      *time.Time
	RawDeadlinePhrase   string
	DraftPayload        map[string]any
	CleanupAfter        *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type DashboardStats struct {
	ActiveCount    int
	SoonCount      int
	ExpiredCount   int
	ConsumedCount  int
	DiscardedCount int
	DeletedCount   int
}
