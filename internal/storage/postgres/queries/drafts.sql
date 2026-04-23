-- name: CreateDraftSession :one
INSERT INTO draft_sessions (
    trace_id, user_id, chat_id, source_kind, status, source_message_id, feedback_message_id,
    draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: GetDraftSession :one
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE id = $1;

-- name: GetDraftSessionByTraceID :one
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE trace_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetEditableDraftSessionForUser :one
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE user_id = $1 AND status IN ('editing_name', 'editing_date')
ORDER BY updated_at DESC
LIMIT 1;

-- name: SetDraftMessageID :exec
UPDATE draft_sessions
SET draft_message_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: SetDraftEditPromptMessageID :exec
UPDATE draft_sessions
SET edit_prompt_message_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateDraftStatus :exec
UPDATE draft_sessions
SET status = $2,
    edit_prompt_message_id = CASE
        WHEN $2 IN ('ready', 'confirmed', 'canceled', 'deleted', 'failed') THEN NULL
        ELSE edit_prompt_message_id
    END,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateDraftFields :exec
UPDATE draft_sessions
SET draft_name = $2,
    draft_expires_on = $3,
    raw_deadline_phrase = $4,
    status = $5,
    edit_prompt_message_id = CASE
        WHEN $5 = 'ready' THEN NULL
        ELSE edit_prompt_message_id
    END,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateDraftPayload :exec
UPDATE draft_sessions
SET draft_payload = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: ApplyDraftAIReviewIfReady :execrows
UPDATE draft_sessions
SET draft_name = $2,
    draft_expires_on = $3,
    raw_deadline_phrase = $4,
    draft_payload = $5,
    updated_at = NOW()
WHERE id = $1
  AND status = 'ready';

-- name: ListStaleDraftSessions :many
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE status IN ('ready', 'editing_name', 'editing_date')
  AND created_at <= $1
ORDER BY created_at ASC;

-- name: LockDraftForConfirmation :one
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE id = $1
FOR UPDATE;

-- name: MarkDraftConfirmed :exec
UPDATE draft_sessions
SET status = $2,
    confirmed_product_id = $3,
    edit_prompt_message_id = NULL,
    updated_at = NOW()
WHERE id = $1;
