# Telegram E2E Assets

This directory stores Shelfy-owned assets for Telegram e2e testing.

## Layout

- `scenarios/` contains interaction scenarios executed by the sibling
  `telegram-bot-e2e-test-tool` repository.
- `date-cases.txt` is a plain text matrix for parser-oriented Telegram input
  checks. It is intentionally separate from scenario JSONL files because the
  same runner can send many product/date phrases through one reusable text
  matrix flow.

Generic orchestration, fixtures, transcripts, and reporting stay outside this
product repo in `../telegram-bot-e2e-test-tool`.
