-- +goose Up

ALTER TABLE draft_sessions
ADD COLUMN IF NOT EXISTS edit_prompt_message_id BIGINT;

-- +goose Down

ALTER TABLE draft_sessions DROP COLUMN IF EXISTS edit_prompt_message_id;
