-- name: GetClockOverride :one
SELECT override_now
FROM app_clock
WHERE singleton = TRUE;

-- name: SetClockOverride :exec
UPDATE app_clock
SET override_now = $1, updated_at = NOW()
WHERE singleton = TRUE;
