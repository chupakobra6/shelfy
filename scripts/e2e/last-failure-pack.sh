#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

TOOL_ROOT="${TOOL_ROOT:-$ROOT_DIR/../telegram-bot-e2e-test-tool}"
FAILURE_JSON="$TOOL_ROOT/artifacts/transcripts/last-failure.json"
FAILURE_TXT="$TOOL_ROOT/artifacts/transcripts/last-failure.txt"
PACK_ROOT="$ROOT_DIR/tmp/e2e-failure-pack"
MAX_LINES_PER_SERVICE="${MAX_LINES_PER_SERVICE:-133}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

require_cmd python3

if [[ ! -d "$TOOL_ROOT" ]]; then
  echo "tool root not found: $TOOL_ROOT" >&2
  exit 1
fi

if [[ ! -x "./scripts/e2e/trace-logs.sh" ]]; then
  echo "trace log helper not found or not executable: ./scripts/e2e/trace-logs.sh" >&2
  exit 1
fi

if [[ ! -f "$FAILURE_JSON" ]]; then
  echo "last failure artifact not found: $FAILURE_JSON" >&2
  exit 1
fi

readarray -t meta < <(FAILURE_JSON="$FAILURE_JSON" PACK_ROOT="$PACK_ROOT" python3 - <<'PY'
import json
import os
from datetime import datetime, timezone
from pathlib import Path

failure = json.loads(Path(os.environ["FAILURE_JSON"]).read_text())
pack_root = Path(os.environ["PACK_ROOT"])
label = str(failure.get("transcript_label") or failure.get("scenario_path") or "failure").strip()
safe_label = "".join(ch if ch.isalnum() or ch in "-._" else "-" for ch in label)
timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
pack_dir = pack_root / f"{timestamp}-{safe_label}"
pack_dir.mkdir(parents=True, exist_ok=True)
print(str(pack_dir))
print(str(failure.get("transcript_label", "")))
print(str(failure.get("failure_at", "")))
PY
)

PACK_DIR="${meta[0]}"
SCENARIO_LABEL="${meta[1]}"
FAILURE_AT="${meta[2]}"

cp "$FAILURE_JSON" "$PACK_DIR/tool-last-failure.json"
if [[ -f "$FAILURE_TXT" ]]; then
  cp "$FAILURE_TXT" "$PACK_DIR/tool-last-failure.txt"
fi

for svc in telegram-api pipeline-worker scheduler-worker; do
  ./scripts/e2e/trace-logs.sh \
    --scenario-label "$SCENARIO_LABEL" \
    --service "$svc" \
    --max-lines "$MAX_LINES_PER_SERVICE" \
    > "$PACK_DIR/trace-logs.${svc}.txt"
done

PACK_DIR="$PACK_DIR" FAILURE_JSON="$FAILURE_JSON" FAILURE_AT="$FAILURE_AT" python3 - <<'PY'
import json
import os
from datetime import datetime, timezone
from pathlib import Path

pack_dir = Path(os.environ["PACK_DIR"])
failure = json.loads(Path(os.environ["FAILURE_JSON"]).read_text())
manifest = {
    "generated_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    "failure_at": os.environ.get("FAILURE_AT", ""),
    "transcript_label": failure.get("transcript_label"),
    "scenario_path": failure.get("scenario_path"),
    "files": {
        "tool_last_failure_json": str(pack_dir / "tool-last-failure.json"),
        "tool_last_failure_txt": str(pack_dir / "tool-last-failure.txt"),
        "telegram_api_logs": str(pack_dir / "trace-logs.telegram-api.txt"),
        "pipeline_worker_logs": str(pack_dir / "trace-logs.pipeline-worker.txt"),
        "scheduler_worker_logs": str(pack_dir / "trace-logs.scheduler-worker.txt"),
    },
}
(pack_dir / "manifest.json").write_text(json.dumps(manifest, indent=2))
print(pack_dir)
PY
