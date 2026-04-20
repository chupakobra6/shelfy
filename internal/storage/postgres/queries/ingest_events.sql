-- name: CreateIngestEvent :execrows
INSERT INTO ingest_events (trace_id, user_id, chat_id, message_id, message_kind, status, summary, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateIngestStatus :exec
UPDATE ingest_events
SET status = $2, summary = $3, updated_at = NOW()
WHERE trace_id = $1;
