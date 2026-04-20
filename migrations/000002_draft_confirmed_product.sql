-- +goose Up

ALTER TABLE draft_sessions
ADD COLUMN IF NOT EXISTS confirmed_product_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_draft_sessions_confirmed_product_id
ON draft_sessions(confirmed_product_id)
WHERE confirmed_product_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_draft_sessions_confirmed_product_id;
ALTER TABLE draft_sessions DROP COLUMN IF EXISTS confirmed_product_id;
