CREATE TABLE IF NOT EXISTS user_settings (
    user_id BIGINT PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'Europe/Moscow',
    digest_local_time TEXT NOT NULL DEFAULT '09:00',
    dashboard_message_id BIGINT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    expires_on DATE NOT NULL,
    raw_deadline_phrase TEXT,
    status TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS draft_sessions (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    chat_id BIGINT NOT NULL,
    source_kind TEXT NOT NULL,
    status TEXT NOT NULL,
    source_message_id BIGINT,
    draft_message_id BIGINT,
    feedback_message_id BIGINT,
    draft_name TEXT,
    draft_expires_on DATE,
    raw_deadline_phrase TEXT,
    draft_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    cleanup_after TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_product_id BIGINT,
    edit_prompt_message_id BIGINT
);

CREATE TABLE IF NOT EXISTS jobs (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT,
    payload JSONB NOT NULL,
    run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS digest_messages (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    telegram_message_id BIGINT NOT NULL,
    state TEXT NOT NULL,
    product_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ingest_events (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    user_id BIGINT,
    chat_id BIGINT,
    message_id BIGINT,
    message_kind TEXT NOT NULL,
    status TEXT NOT NULL,
    summary TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS app_clock (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
    override_now TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (singleton)
);
