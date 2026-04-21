#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

TOOL_ROOT="${TOOL_ROOT:-$ROOT_DIR/../telegram-bot-e2e-test-tool}"
TRACE_ID=""
UPDATE_ID=""
JOB_ID=""
SCENARIO_LABEL=""
SINCE=""
UNTIL=""
SERVICE=""
MAX_LINES="${MAX_LINES:-200}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

usage() {
  cat <<'EOF' >&2
usage: scripts/e2e/trace-logs.sh [--trace-id ID] [--update-id ID] [--job-id ID] [--scenario-label LABEL] [--service NAME] [--since RFC3339] [--until RFC3339] [--max-lines N]
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --trace-id)
      TRACE_ID="${2:-}"
      shift 2
      ;;
    --update-id)
      UPDATE_ID="${2:-}"
      shift 2
      ;;
    --job-id)
      JOB_ID="${2:-}"
      shift 2
      ;;
    --scenario-label)
      SCENARIO_LABEL="${2:-}"
      shift 2
      ;;
    --since)
      SINCE="${2:-}"
      shift 2
      ;;
    --until)
      UNTIL="${2:-}"
      shift 2
      ;;
    --service)
      SERVICE="${2:-}"
      shift 2
      ;;
    --max-lines)
      MAX_LINES="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

require_cmd docker
require_cmd python3

if [[ ! -d "$TOOL_ROOT" ]]; then
  echo "tool root not found: $TOOL_ROOT" >&2
  exit 1
fi

if [[ -n "$SCENARIO_LABEL" ]] && [[ -z "$SINCE" || -z "$UNTIL" ]]; then
  mapfile -t meta < <(TOOL_ROOT="$TOOL_ROOT" SCENARIO_LABEL="$SCENARIO_LABEL" python3 - <<'PY'
import json
import os
from datetime import datetime, timedelta, timezone
from pathlib import Path

tool_root = Path(os.environ["TOOL_ROOT"])
label = os.environ["SCENARIO_LABEL"].strip()
failure_path = tool_root / "artifacts" / "transcripts" / "last-failure.json"
summary_path = tool_root / "artifacts" / "transcripts" / "last-run-summary.json"

def parse_ts(value: str):
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))

def normalize_label(value: str):
    value = value.strip()
    if not value:
        return value
    return value.removesuffix(".jsonl")

def matches(row):
    candidates = {
        normalize_label(str(row.get("transcript_label", ""))),
        normalize_label(Path(str(row.get("scenario_path", ""))).stem),
        normalize_label(Path(str(row.get("scenario_path", ""))).name),
    }
    return normalize_label(label) in candidates

row = None
if failure_path.exists():
    failure = json.loads(failure_path.read_text())
    if matches(failure):
      row = failure

if row is None and summary_path.exists():
    rows = json.loads(summary_path.read_text())
    for item in rows:
        if matches(item):
            row = item
            break

if row is None:
    raise SystemExit(f"scenario label not found in tool artifacts: {label}")

failure_at = parse_ts(str(row.get("failure_at", "")))
finished_at = parse_ts(str(row.get("finished_at", "")))
anchor = failure_at or finished_at
if anchor is None:
    raise SystemExit("scenario metadata has no failure_at or finished_at timestamp")
since = anchor - timedelta(seconds=45)
until = anchor + timedelta(seconds=45)
print(since.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"))
print(until.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"))
PY
  )
  if [[ ${#meta[@]} -ge 2 ]]; then
    SINCE="${meta[0]}"
    UNTIL="${meta[1]}"
  fi
fi

if [[ -z "$SINCE" ]]; then
  SINCE="$(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) - timedelta(seconds=45)).isoformat().replace("+00:00", "Z"))
PY
)"
fi
if [[ -z "$UNTIL" ]]; then
  UNTIL="$(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) + timedelta(seconds=45)).isoformat().replace("+00:00", "Z"))
PY
)"
fi

services=("telegram-api" "pipeline-worker" "scheduler-worker")
if [[ -n "$SERVICE" ]]; then
  services=("$SERVICE")
fi

emit_service_logs() {
  local svc="$1"
  docker compose logs --no-color --since "$SINCE" --until "$UNTIL" "$svc" 2>/dev/null | \
    TRACE_ID="$TRACE_ID" UPDATE_ID="$UPDATE_ID" JOB_ID="$JOB_ID" SERVICE="$svc" MAX_LINES="$MAX_LINES" python3 -c '
import json
import os
import re
import sys

trace_id = os.environ.get("TRACE_ID", "").strip()
update_id = os.environ.get("UPDATE_ID", "").strip()
job_id = os.environ.get("JOB_ID", "").strip()
max_lines = int(os.environ.get("MAX_LINES", "200"))
ansi_re = re.compile(r"\x1b\[[0-9;]*m")
space_re = re.compile(r"\s+")
filters = [(k, v) for k, v in (("trace_id", trace_id), ("update_id", update_id), ("job_id", job_id)) if v]
count = 0

def compact(value: str, limit: int = 160) -> str:
    value = ansi_re.sub("", value)
    value = space_re.sub(" ", value).strip()
    if len(value) <= limit:
        return value
    return value[: limit - 1] + "…"

for raw in sys.stdin:
    if count >= max_lines:
        break
    line = ansi_re.sub("", raw.rstrip("\n"))
    if "|" in line:
        _, line = line.split("|", 1)
    line = line.strip()
    if not line:
        continue
    normalized = ""
    try:
        record = json.loads(line)
    except Exception:
        if filters:
            continue
        normalized = compact(line, 200)
    else:
        if filters:
            matched = False
            for key, expected in filters:
                value = record.get(key)
                if value is not None and str(value) == expected:
                    matched = True
                    break
            if not matched:
                continue
        keys = ("time", "level", "msg", "trace_id", "update_id", "job_id", "error")
        parts = []
        for key in keys:
            value = record.get(key)
            if value in (None, ""):
                continue
            parts.append(f"{key}={compact(str(value), 120)}")
        normalized = " ".join(parts)
    if not normalized:
        continue
    print(normalized)
    count += 1
'
}

for svc in "${services[@]}"; do
  if [[ -z "$SERVICE" ]]; then
    echo "== ${svc} =="
  fi
  emit_service_logs "$svc"
done
