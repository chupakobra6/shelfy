-- name: EnqueueJob :execrows
INSERT INTO jobs (trace_id, job_type, status, idempotency_key, payload, run_at)
VALUES ($1, $2, 'queued', $3, $4, $5)
ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING;

-- name: ClaimJob :one
WITH next_job AS (
    SELECT jobs.id
    FROM jobs
    WHERE jobs.status IN ('queued', 'retry')
      AND jobs.run_at <= $1
      AND jobs.job_type = ANY($2::text[])
    ORDER BY jobs.run_at ASC, jobs.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs
SET status = 'running',
    attempts = jobs.attempts + 1,
    claimed_at = NOW(),
    updated_at = NOW(),
    last_error = NULL
FROM next_job
WHERE jobs.id = next_job.id
RETURNING jobs.id, jobs.trace_id, jobs.job_type, jobs.status, jobs.payload, jobs.run_at, jobs.attempts, jobs.max_attempts, jobs.idempotency_key, jobs.last_error;

-- name: CountActiveJobsUpTo :one
SELECT COUNT(*)::bigint
FROM jobs
WHERE status IN ('queued', 'retry', 'running')
  AND run_at <= $1
  AND job_type = ANY($2::text[]);

-- name: MarkJobDone :exec
UPDATE jobs
SET status = 'done', completed_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: MarkJobRetry :exec
UPDATE jobs
SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'retry' END,
    run_at = $2,
    last_error = $3,
    updated_at = NOW()
WHERE id = $1;
