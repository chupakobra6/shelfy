# AGENTS.md

## Project overview
- This repository contains the Shelfy product code and product-owned local tooling.
- Keep changes minimal and targeted unless the task clearly requires a structural change.
- Preserve the current architecture unless the user asks for a deeper redesign.

## Source of truth
- Follow nearby code patterns first.
- Treat these files as the canonical entrypoints for behavior:
  - `cmd/telegram-api/main.go` for the main bot process
  - `internal/bot` for Telegram UX flow, callbacks, and dashboard behavior
  - `internal/ingest` for text/audio parsing and draft creation
  - `internal/scheduler` for digests, cleanup, and dev control endpoints
  - `internal/storage/postgres` for persisted behavior and query contracts
- Treat `assets/copy/runtime.ru.yaml` as the canonical runtime copy catalog.
- Treat `docs/message-inventory.ru.yaml` as the metadata/source-of-truth companion for copy work, not as a second runtime catalog.

## Project constraints
- Keep generic Telegram E2E orchestration in the sibling `telegram-bot-e2e-test-tool` repo.
- Keep only product-owned E2E assets here:
  - scenarios under `e2e/telegram/scenarios/`
  - parsing corpora such as `e2e/telegram/date-cases.txt`
  - runtime-specific diagnostics such as `cmd/e2e-triage`
- Keep runtime copy under `assets/`, not in `docs/` or `internal/`.
- Prefer the existing Go-based `vosk-transcribe` command path; do not reintroduce repo-owned Python helpers for ASR tooling.

## Commands
- Install/update dependencies: `make setup`
- Build the heavy runtime layer once: `make runtime-base`
- Rebuild the heavy runtime layer intentionally: `make runtime-base-rebuild`
- Start local runtime: `make dev`
- Run tests: `make test`
- Run lint: `make lint`
- Regenerate typed SQL code: `make generate`
- Triage the latest E2E failure pack: `make e2e-last-failure`
- Slice recent service logs for a scenario or trace: `make e2e-trace-logs`

## Verification
- Run the narrowest relevant validation first.
- If Go behavior changed materially, run at least:
  - `go test ./...`
- If runtime startup, Docker wiring, or local commands changed, also check:
  - `make help`
- Before concluding that live bot behavior is "missing" or that current-head code is not working, verify the running local stack is actually fresh:
  - check container/process age,
  - check expected job or payload markers,
  - and prefer log/DB evidence over chat-only impressions.
- After cross-repo Shelfy/e2e contract changes, prefer one current-head integrated rerun of the relevant end-to-end path instead of stopping at package-only greens.
- If Telegram UI copy, button labels/order, dashboard flows, or control API behavior changes, sync the sibling `telegram-bot-e2e-test-tool` suite/docs in the same pass and finish with one live `make run-suite` there.

## Documentation boundaries
- Keep tracked docs in English unless the user explicitly wants another language.
- Keep reusable setup and runtime guidance in `README.md` and `docs/*.md`.
- Keep repo-specific agent guidance here concise and stable.
