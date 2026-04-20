# Timed Testing and Dev Controls

Shelf time-based logic is testable without waiting for real clock time.

## Control endpoints

These endpoints are available only when:

- `SHELFY_ENABLE_DEV_CONTROL_API=true`
- environment is not production

## Endpoints

- `POST /control/time/set`
  - body: `{ "now": "2026-04-13T06:00:00Z" }`
- `POST /control/time/advance`
  - body: `{ "duration": "24h" }`
- `POST /control/time/clear`
- `POST /control/jobs/run-due`
  - body: `{ "limit": 50, "include_maintenance": true }`
  - runs maintenance tick first by default, then drains due scheduler jobs immediately
- `POST /control/digests/reconcile`

## Suggested flow for E2E

1. Set virtual time to a known baseline.
2. Drive Telegram interactions.
3. Advance time.
4. Trigger due jobs with `/control/jobs/run-due` so digest and cleanup jobs execute deterministically.
5. Assert digest appearance or cleanup behavior.
