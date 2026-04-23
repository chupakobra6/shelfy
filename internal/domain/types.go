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
	JobTypeRefineDraftAI     = "refine_draft_ai"
	JobTypeDeleteMessages    = "delete_messages"
	JobTypeSendMorningDigest = "send_morning_digest"
)

const (
	DraftPayloadKeyAIReviewStatus       = "ai_review_status"
	DraftPayloadKeyFastSource           = "fast_source"
	DraftPayloadKeyFastConfidence       = "fast_confidence"
	DraftPayloadKeySmartReviewAttempted = "smart_review_attempted"
	DraftPayloadKeyRawTranscript        = "raw_transcript"
	DraftPayloadKeyNormalizedTranscript = "normalized_transcript"
	DraftPayloadKeyOriginalText         = "original_text"
	DraftPayloadKeyReviewCleanedText    = "review_cleaned_text"
	DraftPayloadKeyReviewReasonCode     = "review_reason_code"
	DraftPayloadKeyReviewApplyReason    = "review_apply_reason"
	DraftPayloadKeyReviewApplied        = "review_applied"
)

const (
	AIReviewStatusPending  = "pending"
	AIReviewStatusImproved = "improved"
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
