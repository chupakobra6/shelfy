# Shelfy

Shelfy is a personal Telegram bot for tracking product expiration dates with a low-noise UX, background ingestion workers, and a pinned dashboard.

## Runtime

- `migrate`
  Applies Goose migrations once before long-running services start.
- `telegram-api`
  Receives Telegram updates, handles `/start`, dashboard callbacks, and enqueues background jobs.
- `pipeline-worker`
  Processes text/photo/audio ingestion jobs, OCR/ASR/model parsing, and creates draft sessions.
- `scheduler-worker`
  Handles morning digests, staged cleanup, transient message deletion, and non-production control endpoints.
- `postgres`
  Stores application state, jobs, drafts, and debug metadata.

## Development

```bash
cp .env.example .env
make setup
make dev
```

For local development the bot expects a native Ollama server on the host, reachable from Docker at `http://host.docker.internal:11434`.
For Russian voice input use a multilingual Whisper model such as `ggml-base.bin`, not `ggml-base.en.bin`.
The pipeline worker expects Whisper models in `./models`, mounted into the container as `/models`.
If outbound network access requires a proxy, add standard `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` / `NO_PROXY` variables to `.env` so the containers inherit them.
Use `make logs` to watch structured runtime logs after `make dev`.
When you change storage queries or the schema snapshot, regenerate typed DB code with `make generate`.

## Notes

- Runtime user-facing copy lives in `copy/runtime.ru.yaml`.
- Copy-generation requirements live in `docs/copy-spec.md`, and the message inventory for copy work lives in `docs/message-inventory.ru.yaml`.
- Timed test controls are exposed only when `SHELFY_ENABLE_DEV_CONTROL_API=true`.
- `sqlc` query sources live in `internal/storage/postgres/queries`, and generated code is committed in `internal/storage/postgres/sqlcgen`.
