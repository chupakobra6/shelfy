-- name: UpsertUserSettings :execrows
INSERT INTO user_settings (user_id, chat_id, timezone, digest_local_time, dashboard_message_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE SET
    chat_id = EXCLUDED.chat_id,
    timezone = EXCLUDED.timezone,
    digest_local_time = EXCLUDED.digest_local_time,
    dashboard_message_id = COALESCE(EXCLUDED.dashboard_message_id, user_settings.dashboard_message_id),
    updated_at = NOW();

-- name: SetDashboardMessageID :exec
UPDATE user_settings
SET dashboard_message_id = $2, updated_at = NOW()
WHERE user_id = $1;

-- name: UpdateUserTimezone :exec
UPDATE user_settings
SET timezone = $2, updated_at = NOW()
WHERE user_id = $1;

-- name: UpdateUserDigestLocalTime :exec
UPDATE user_settings
SET digest_local_time = $2, updated_at = NOW()
WHERE user_id = $1;

-- name: GetUserSettings :one
SELECT user_id, chat_id, timezone, digest_local_time, dashboard_message_id
FROM user_settings
WHERE user_id = $1;

-- name: ListUsers :many
SELECT user_id, chat_id, timezone, digest_local_time, dashboard_message_id
FROM user_settings
WHERE active = TRUE
ORDER BY user_id ASC;

-- name: DashboardStats :one
SELECT
    COUNT(*) FILTER (WHERE status = 'active')::bigint AS active_count,
    COUNT(*) FILTER (WHERE status = 'active' AND expires_on >= $2::date AND expires_on <= $3::date)::bigint AS soon_count,
    COUNT(*) FILTER (WHERE status = 'active' AND expires_on < $2::date)::bigint AS expired_count,
    COUNT(*) FILTER (WHERE status = 'consumed')::bigint AS consumed_count,
    COUNT(*) FILTER (WHERE status = 'discarded')::bigint AS discarded_count,
    COUNT(*) FILTER (WHERE status = 'deleted')::bigint AS deleted_count
FROM products
WHERE user_id = $1;
