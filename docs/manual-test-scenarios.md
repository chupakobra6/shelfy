# Shelfy Manual Test Scenarios

This document is the baseline manual test checklist for local Telegram testing.

## Setup

1. Start the stack with `make dev`.
2. Open live logs with `make logs`.
3. Make sure the bot answers in a private chat.

## Core Start Flow

1. Send `/start`.
2. Verify a new pinned dashboard is created.
3. Send `/start` again.
4. Verify the old dashboard is replaced by a new pinned dashboard.
5. Press buttons on the old dashboard if it still exists.
6. Verify the bot shows the stale-dashboard feedback instead of changing state.

## Text Fast Path

1. Send `зефир завтра`.
2. Verify there is no long-lived `processing` message.
3. Verify a draft card appears quickly.
4. Confirm the product.
5. Verify the source text, draft card, and transient confirmation disappear in the expected order.

## Text Date Variants

Test each input as a new product:

- `молоко до пятницы`
- `молоко до пт`
- `молоко до пятницаы`
- `кефир 1 мая`
- `йогурт 26`
- `сыр 14.04`

For each case verify:

1. The product name is correct.
2. The date is correct.
3. No unnecessary `processing` message remains in chat.

## Name-Only Draft

1. Send `молоко`.
2. Verify the bot creates an incomplete draft without waiting for the worker.
3. Verify the draft shows that the expiry date is missing.

## Draft Edit Name

1. Create an incomplete draft.
2. Press `📝 Название`.
3. Verify only one edit prompt is visible.
4. Send a valid name.
5. Verify both your reply and the prompt are deleted immediately.
6. Verify the draft card updates in place.

## Draft Edit Date

1. Create an incomplete or editable draft.
2. Press `📅 Срок`.
3. Press `📅 Срок` several times quickly.
4. Verify old prompts do not pile up.
5. Send `сб`.
6. Verify the draft updates and your message plus the prompt disappear immediately.

## Draft Invalid Date

1. Press `📅 Срок`.
2. Send an invalid value like `ываф`.
3. Verify your invalid message disappears quickly.
4. Verify the invalid-date feedback disappears after a short delay.
5. Verify the draft remains editable.

## Unsupported Message

1. Send a sticker.
2. Verify the unsupported-type feedback appears.
3. Verify the sticker message is removed quickly.
4. Verify the feedback disappears after a short delay.

## Photo Pipeline

1. Send a clear package photo with a readable expiry date.
2. Verify `processing` appears only for the background path.
3. Verify `processing` disappears as soon as the draft is ready.
4. Verify the draft card contains the product name and expiry.

## Audio Pipeline

1. Send a voice message like `добавь кефир до пятницы`.
2. Verify `processing` appears.
3. Verify it disappears when the draft is ready or on failure.
4. Verify the draft contents are reasonable.

## Product Closure

1. Confirm a product.
2. Open it from the dashboard list.
3. Mark it as `✅ Съедено`.
4. Verify it disappears from visible lists.
5. Verify it is still counted in statistics.

## Timed Controls

1. Use the dev control API to set virtual time close to the digest time.
2. Trigger due jobs.
3. Verify the morning digest appears.
4. Close or delete the referenced products.
5. Trigger digest reconciliation.
6. Verify the digest message is removed.

## Log Review

During all scenarios verify that logs remain readable:

- no maintenance spam at `INFO`
- no long-lived Telegram send timeouts on simple text flows
- immediate text fast-path requests do not enqueue `ingest_text`
- delayed delete jobs are used only for true delayed cleanup
