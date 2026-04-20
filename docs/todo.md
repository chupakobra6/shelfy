# Shelfy TODO

## P0

- Stabilize single-item UX after the text fast-path rollout.
- Verify immediate cleanup behavior for:
  - draft edit prompts
  - successful name/date edits
  - unsupported message feedback
  - async processing feedback
- Add regression tests around text fast-path and prompt cleanup.

## P1

- Improve multimodal extraction:
  - pass cleaned OCR text into the vision fallback prompt
  - compare `OCR -> Gemma text` against `Gemma vision only` on a real photo set
- Add retry/backoff tuning for transient Telegram API failures.
- Add a clearer loading state for slow photo/audio pipelines without polluting chat history.

## P2

- Batch add from multi-line text.
- Batch add from one voice note with several products.
- Batch add from multiple photos / album.
- Receipt ingestion flow.

## Research

- Measure whether `Gemma 3` vision alone beats `Tesseract + Gemma text` on Russian упаковки.
- Decide whether OCR should remain the primary path or become a cheap hint for vision.
- Evaluate whether Telegram-side request routing should move to a dedicated adapter with per-method retry policy.
