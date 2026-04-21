-- name: ListDraftSessionsForUser :many
SELECT id, trace_id, user_id, chat_id, source_kind, status, source_message_id, draft_message_id, feedback_message_id,
       draft_name, draft_expires_on, raw_deadline_phrase, draft_payload, cleanup_after, created_at, updated_at,
       confirmed_product_id, edit_prompt_message_id
FROM draft_sessions
WHERE user_id = $1
ORDER BY id ASC;

-- name: DeleteDraftSessionsByUser :execrows
DELETE FROM draft_sessions
WHERE user_id = $1;

-- name: ListActiveDigestMessagesForUser :many
SELECT id, user_id, telegram_message_id, state, product_ids, created_at, updated_at
FROM digest_messages
WHERE user_id = $1 AND state = 'active'
ORDER BY id ASC;

-- name: DeleteDigestMessagesByUser :execrows
DELETE FROM digest_messages
WHERE user_id = $1;

-- name: DeleteProductsByUser :execrows
DELETE FROM products
WHERE user_id = $1;

-- name: DeleteJobsForE2EUser :execrows
DELETE FROM jobs
WHERE payload @> jsonb_build_object('user_id', to_jsonb($1::bigint))
   OR ($2::bigint <> 0 AND payload @> jsonb_build_object('chat_id', to_jsonb($2::bigint)));

-- name: ResetUserSettingsForE2E :exec
UPDATE user_settings
SET dashboard_message_id = NULL,
    timezone = $2,
    digest_local_time = $3,
    updated_at = NOW()
WHERE user_id = $1;
