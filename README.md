# Shelfy

Shelfy is a personal Telegram bot for tracking product expiration dates with a low-noise UX.

The bot is built around one pinned dashboard, transient draft cards, and background pipelines for text, photo, and audio ingestion. The goal is to keep the chat clean while still making parsing, edits, digests, and cleanup observable and testable.

## Highlights

- one pinned dashboard instead of a command-heavy chat UI
- text, photo, voice, and audio ingestion
- transient drafts before save, with cleanup-first UX
- morning digests and deterministic timed testing through dev controls
- real Telegram E2E scenarios kept next to the product

## What it does

- keeps one pinned dashboard as the main control surface
- accepts new products from normal incoming messages, not slash commands
- builds transient draft cards before saving products
- parses text, photos, voice messages, and audio in background workers
- sends morning digests and cleans up transient chat noise over time
- supports deterministic timed testing through non-production dev controls

## Stack

- Go services
- PostgreSQL
- Docker Compose for local runtime
- Ollama on the host for LLM inference
- Tesseract for OCR
- `whisper.cpp` for speech-to-text

## Architecture

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

More detail lives in [docs/architecture.md](./docs/architecture.md).

## Quick start

```bash
cp .env.example .env
make setup
make dev
```

For a quick command overview:

```bash
make help
```

Useful next reads:

- [docs/architecture.md](./docs/architecture.md)
- [docs/copy-spec.md](./docs/copy-spec.md)
- [docs/manual-test-scenarios.md](./docs/manual-test-scenarios.md)

## Local runtime expectations

For local development the bot expects a native Ollama server on the host, reachable from Docker at `http://host.docker.internal:11434`.
For Russian voice input use a multilingual Whisper model such as `ggml-base.bin`, not `ggml-base.en.bin`.
The pipeline worker expects Whisper models in `./models`, mounted into the container as `/models`.
If outbound network access requires a proxy, add standard `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` / `NO_PROXY` variables to `.env` so the containers inherit them.

## Main commands

```bash
make help
make dev
make down
make logs
make test
make generate
```

Use `make logs` to watch structured runtime logs after `make dev`. When you change storage queries or the schema snapshot, regenerate typed DB code with `make generate`.

## Telegram E2E

`Shelfy` keeps only Telegram E2E assets in this repo:

- scenario files under `e2e/telegram/scenarios/`
- text case corpora such as `e2e/telegram/date-cases.txt`

All execution, orchestration, fixtures, transcripts, and helper commands live in the sibling `telegram-bot-e2e-test-tool` repository.

Typical flows:

```bash
make -C ../telegram-bot-e2e-test-tool fixtures
make -C ../telegram-bot-e2e-test-tool run-scenario \
  CHAT=@your_bot_username \
  SCENARIO="$PWD/e2e/telegram/scenarios/01-start-and-stale-dashboard.jsonl $PWD/e2e/telegram/scenarios/02-dashboard-navigation-and-settings.jsonl"

make -C ../telegram-bot-e2e-test-tool run-text-matrix \
  CHAT=@your_bot_username \
  CASES="$PWD/e2e/telegram/date-cases.txt"
```

## Repository map

- Runtime user-facing copy lives in `copy/runtime.ru.yaml`.
- Copy-generation requirements live in `docs/copy-spec.md`, and the message inventory for copy work lives in `docs/message-inventory.ru.yaml`.
- Product backlog lives in `docs/todo.md`.
- Manual Telegram verification scenarios live in `docs/manual-test-scenarios.md`.
- Automated Telegram E2E scenarios live in `e2e/telegram/scenarios/`.
- Text matrices for parsing regressions live in `e2e/telegram/date-cases.txt`.
- Timed test controls are exposed only when `SHELFY_ENABLE_DEV_CONTROL_API=true`.
- `sqlc` query sources live in `internal/storage/postgres/queries`, and generated code is committed in `internal/storage/postgres/sqlcgen`.
