-- +goose Up

ALTER TABLE user_settings
    DROP COLUMN IF EXISTS dashboard_origin_page,
    DROP COLUMN IF EXISTS dashboard_origin_view,
    DROP COLUMN IF EXISTS dashboard_product_id,
    DROP COLUMN IF EXISTS dashboard_page,
    DROP COLUMN IF EXISTS dashboard_view;

-- +goose Down

ALTER TABLE user_settings
    ADD COLUMN IF NOT EXISTS dashboard_view TEXT NOT NULL DEFAULT 'home',
    ADD COLUMN IF NOT EXISTS dashboard_page INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS dashboard_product_id BIGINT,
    ADD COLUMN IF NOT EXISTS dashboard_origin_view TEXT,
    ADD COLUMN IF NOT EXISTS dashboard_origin_page INTEGER;

UPDATE user_settings
SET dashboard_view = 'home',
    dashboard_page = 0,
    dashboard_product_id = NULL,
    dashboard_origin_view = NULL,
    dashboard_origin_page = NULL
WHERE dashboard_view IS NULL;
