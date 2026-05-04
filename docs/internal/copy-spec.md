# Shelfy Copy Spec

This document is intended for a separate model that will generate all user-facing Russian copy for Shelfy.

Runtime strings live in `assets/copy/runtime.ru.yaml`.
Message metadata and scenario inventory live in `docs/internal/message-inventory.ru.yaml`.

## Product context

- Shelfy is a personal Telegram bot for tracking product expiration dates.
- The UX is intentionally low-noise: one pinned dashboard, short transient cards, minimal chat pollution.
- The bot is helpful, precise, calm, and practical.

## Tone of voice

- Friendly and clear, not chatty.
- Never robotic.
- Never vague.
- Always action-oriented.
- Always explain the next user action in one short sentence if the flow needs input.

## Emoji rules

- Emoji are required in user-facing messages.
- Use 1-2 emoji per message block, not more.
- Emoji must support meaning, not decorate randomly.
- Prefer food, time, checklist, warning, confirmation, inbox, and cleanup-adjacent emoji.

## Button style

- Buttons should be short.
- Buttons should be imperative or state-based.
- Avoid long button text.
- Prefer 1-2 words.

## Message length

- Dashboard text should fit comfortably on one phone screen.
- Draft cards should stay under 8 lines when possible.
- Error and unsupported-type feedback should usually fit in 1-3 short lines.
- Digest messages should be scannable and grouped.

## Lists

- Use bullets for multiple products only when that improves scanning.
- Keep per-line density low.
- If a list has dates, align the date pattern consistently.

## Error message rules

- State what happened.
- State what the user can do next.
- Do not blame the user.
- Do not expose technical internals.

## Reminder and digest rules

- Morning digest should feel useful, not alarming by default.
- Expired products can be more direct.
- “Soon to expire” should be softer than “expired”.
- Encourage closure actions: eat, discard, or delete.

## Ambiguity rules

- If a draft is incomplete, say exactly which field is missing.
- If date interpretation is ambiguous, ask for clarification instead of pretending certainty.
- Do not invent product names or dates that were not extracted confidently.

## Catalog contract

Every catalog entry must preserve:

- `message_id`
- where it appears
- when it appears
- expected user action
- whether the message is auto-deleted
- interpolation fields
- button references

The generated copy must remain compatible with these fields.

Do not put operational metadata back into `assets/copy/runtime.ru.yaml`.
That file is runtime-only and should contain just labels and message templates.
