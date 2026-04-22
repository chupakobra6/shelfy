# Shelfy

Shelfy is a personal Telegram bot for tracking product expiration dates with a low-noise UX.

The bot is built around one pinned dashboard, transient draft cards, and background pipelines for text, photo, and audio ingestion. The goal is to keep the chat clean while still making parsing, edits, digests, and cleanup observable and testable.

## Highlights

- one pinned dashboard instead of a command-heavy chat UI
- paginated dashboard product lists instead of unbounded keyboards
- text, photo, voice, and audio ingestion
- multimodal photo intake where a photo caption can complement OCR/image parsing
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
- Vosk `small-ru-0.22` for speech-to-text
- shared runtime base image so app rebuilds do not reinstall OCR/ASR dependencies

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
make runtime-base
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
For Russian voice input the pipeline worker expects a Vosk model directory at `./models/vosk-model-small-ru-0.22`, mounted into the container as `/models/vosk-model-small-ru-0.22`.
The heavy runtime layer is split out into `shelfy-runtime-base:vosk-lib-0.3.45-small-ru-0.22`.
`make dev` and `make up` now only ensure that base image exists; they no longer force a rebuild of the heavy runtime layer on every loop.
Use `make runtime-base-rebuild` only when you intentionally change OCR/ASR/runtime dependencies.
If you run raw `docker compose` commands instead of `make`, build the base image once first with `make runtime-base`.
If outbound network access requires a proxy, add standard `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` / `NO_PROXY` variables to `.env` so the containers inherit them.

Current runtime dependencies are already close to the minimum safe set for existing flows:

- `ffmpeg` for voice/audio transcoding
- `tesseract-ocr` + `tesseract-ocr-rus` for photo OCR
- `libvosk.so` for offline speech-to-text
- `libatomic1` because `libvosk.so` needs it on this platform
- `libstdc++6` because upstream `libvosk.so` links against it

The app image stays thin and does not carry Python; `vosk-transcribe` is now a small Go binary linked against `libvosk`.

## Main commands

```bash
make help
make runtime-base
make runtime-base-rebuild
make dev
make down
make logs
make lint
make test
make generate
```

Use `make logs` to watch structured runtime logs after `make dev`. When you change storage queries or the schema snapshot, regenerate typed DB code with `make generate`.

## Telegram E2E

`Shelfy` keeps only Telegram E2E assets in this repo:

- scenario files under `e2e/telegram/scenarios/`
- text case corpora such as `e2e/telegram/date-cases.txt`

All execution, orchestration, fixtures, transcripts, and helper commands live in the sibling `telegram-bot-e2e-test-tool` repository.

Default triage order after an automated E2E run:

1. open `../telegram-bot-e2e-test-tool/artifacts/transcripts/last-run-summary.txt`
2. if failed, open `../telegram-bot-e2e-test-tool/artifacts/transcripts/last-failure.txt`
3. if still blocked, run `make e2e-last-failure`
4. only then open raw transcripts or full `docker compose logs`

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

Command contract:

- `/start` is bootstrap + idempotent and no longer recreates the dashboard on every repeat.
- `/dashboard` is the explicit recovery/home command for the active dashboard.

Deterministic stateful blocks:

- `POST /control/e2e/reset` is available only in non-production when `SHELFY_ENABLE_DEV_CONTROL_API=true` and `SHELFY_E2E_TEST_USER_ID` is configured.
- Generic block orchestration now lives in the sibling Telegram E2E tool, not in Shelfy.
- Shelfy keeps only product-owned control primitives such as `/control/e2e/reset`, `/control/time/*`, `/control/jobs/run-due`, and `/control/digests/reconcile`.

Example timed setup block:

```bash
make -C ../telegram-bot-e2e-test-tool run-block \
  CHAT=@your_bot_username \
  CONTROL_URL=http://127.0.0.1:8081 \
  RUN_PREFIX=demo123 \
  SCENARIO="$PWD/e2e/telegram/scenarios/00-home-ready.jsonl $PWD/e2e/telegram/scenarios/11-timed-digest-setup.jsonl.tmpl"
```

`00-home-ready` is the bootstrap helper. Some navigation scenarios, such as dashboard-only flows, are intentionally not standalone after a hard reset and should be composed behind `00-home-ready` or `/dashboard` recovery first.

Compact triage helpers in this repo:

```bash
make e2e-last-failure
make e2e-trace-logs TRACE_ID=74ca98dc944f13f4
make e2e-trace-logs SCENARIO_LABEL=02-dashboard-navigation-and-settings
```

`go run ./cmd/e2e-triage trace-logs` slices recent container logs by time and optional `trace_id` / `update_id` / `job_id`.
`go run ./cmd/e2e-triage last-failure-pack` builds a compact pack under `tmp/e2e-failure-pack/` from the latest tool failure artifact and normalized service log slices.

## Repository map

- Runtime user-facing copy lives in `assets/copy/runtime.ru.yaml`.
- Copy-generation requirements live in `docs/copy-spec.md`, and the message inventory for copy work lives in `docs/message-inventory.ru.yaml`.
- Product backlog lives in `docs/todo.md`.
- Manual Telegram verification scenarios live in `docs/manual-test-scenarios.md`.
- Automated Telegram E2E scenarios live in `e2e/telegram/scenarios/`.
- Text matrices for parsing regressions live in `e2e/telegram/date-cases.txt`.
- Timed test controls are exposed only when `SHELFY_ENABLE_DEV_CONTROL_API=true`.
- `sqlc` query sources live in `internal/storage/postgres/queries`, and generated code is committed in `internal/storage/postgres/sqlcgen`.
