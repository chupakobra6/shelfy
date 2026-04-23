# Shelfy Architecture

## Processes

- `migrate`
  - runs Goose SQL migrations once
  - exists before long-running workers start
- `telegram-api`
  - receives Telegram updates
  - handles `/start`
  - edits the pinned dashboard
  - handles inline callback actions
  - enqueues background work into `jobs`
- `pipeline-worker`
  - consumes text/audio ingest jobs
  - runs ASR and deterministic parsing
  - creates `draft_sessions`
  - sends transient draft cards
  - runs one background cleaner pass while a draft is still `ready`
- `scheduler-worker`
  - enqueues due morning digests
  - sends digest messages
  - deletes transient messages on schedule
  - cleans stale drafts
  - exposes non-production control endpoints for time-based testing

## Storage

- `user_settings`
- `products`
- `draft_sessions`
- `jobs`
- `digest_messages`
- `ingest_events`
- `app_clock`

## Time model

- Business time is read through `app_clock` when present.
- Production can leave `override_now` unset.
- Development and E2E tooling can set or advance virtual time.
- `POST /control/jobs/run-due` can run maintenance and then drain runnable scheduler jobs immediately.

## UX model

- One pinned dashboard home screen
- Separate transient draft card
- Minimal chat history through staged deletion
- Digest messages remain separate from the dashboard

## Observability

- structured JSON logging
- every update/job should carry `trace_id`, and where applicable `update_id`, `job_id`, `user_id`, `draft_id`
- failures in ASR/text parsing, queue state transitions, and Telegram side-effects should be visible in logs
