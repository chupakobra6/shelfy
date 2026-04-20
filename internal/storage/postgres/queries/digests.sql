-- name: CreateDigestMessage :execrows
INSERT INTO digest_messages (user_id, telegram_message_id, state, product_ids)
VALUES ($1, $2, 'active', $3);

-- name: ListActiveDigestMessages :many
SELECT id, user_id, telegram_message_id, state, product_ids, created_at, updated_at
FROM digest_messages
WHERE state = 'active';

-- name: MarkDigestDeleted :exec
UPDATE digest_messages
SET state = 'deleted', updated_at = NOW()
WHERE id = $1;
