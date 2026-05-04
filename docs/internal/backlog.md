# Shelfy Backlog

## P0

- Finish single-item UX after the fast text path rollout.
- Verify immediate cleanup behavior for:
  - draft edit prompts;
  - successful name and date edits;
  - unsupported-message feedback;
  - asynchronous `processing` feedback.
- Add regression tests for the fast text path and prompt cleanup.

## P1

- Add retry/backoff for temporary Telegram API failures.
- Add a clearer loading state for slow audio pipeline cases without polluting the chat.

## P2

- Batch add from multiline text.
- Batch add from one voice message with several products.
- Keep focus on text/voice UX and avoid expanding modality scope without a strong new stack.

## Research

- Evaluate whether Telegram-side request routing should move into a separate adapter with per-method retry policy.
