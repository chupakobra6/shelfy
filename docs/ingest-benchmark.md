# Ingest Benchmark

This benchmark now separates two different questions:

- `text`: parser-only regression coverage for typed or transcript-like inputs
- `voice`: public human-speech end-to-end checks for `Vosk -> normalized transcript -> maybe cleaner -> final card`

Synthetic audio fixtures are no longer used. Public media cases are asset-backed: the corpus pins either a repo-local `asset_path` or a downloadable public `download_url` plus checksum and source metadata.

The corpus lives in:

- `internal/ingest/testdata/benchmark_corpus.json`

The main corpus is now `200` cases:

- `100` text cases
- `100` voice cases

Trivial happy-path cases were removed from the main score. Basic one-word or obvious parser correctness is still covered by unit and regression tests, not by the headline benchmark.

Voice now uses a hybrid public corpus:

- `30` repo-local pinned Golos clips under `internal/ingest/testdata/benchmark_audio/`
- `70` dataset-backed Golos rows fetched and checksum-validated on demand

Text coverage was expanded to the same scenario level: branded foods, quantity and package noise, delivery/store contamination, broad-entity shortening cases, date-only partials, and ASR-like phrasing.

## Variants

- `text_fast_only`: deterministic text path without cleaner.
- `text_fast_plus_cleaner`: first fast card plus one cleaner pass over normalized text before the final benchmark outcome is scored.
- `voice_vosk_fast_only`: public audio -> `Vosk` -> normalized transcript -> deterministic parse without cleaner.
- `voice_vosk_fast_plus_cleaner`: the same voice path plus one cleaner pass over normalized transcript text before the final benchmark outcome is scored.

## Run

Download and validate all selected public assets before the run:

```bash
make benchmark-ingest ARGS='-dataset-setup'
```

Portable smoke run with a local HTML audit pack:

```bash
make benchmark-ingest-smoke ARGS='-include voice -dataset-setup'
```

Production-like run with current-head Docker `vosk-transcribe`:

```bash
make benchmark-ingest-prod ARGS='-dataset-setup -emit-report'
```

Filter by case id or tag:

```bash
make benchmark-ingest ARGS='-include voice -dataset-setup'
```

`-dataset-setup` only prefetches the families and filtered cases selected for the current run. If `asset_path` is present, the runner validates the local file and checksum without downloading anything. Dataset-backed Golos cases are downloaded into the local benchmark cache and verified against the pinned SHA256 from the corpus.

## Report

`-emit-report` writes a timestamped audit bundle under `tmp/ingest-benchmark/reports/...` unless `-report-dir` is provided.

Each report contains:

- `index.html`: interactive audit UI
- `results.json`: full per-case outputs
- `summary.json`: aggregate variant metrics
- `audit.json`: machine-readable manual audit queue and suspicious-case rollups
- `audit.md`: audit-first markdown summary for manual benchmark review
- `artifacts/`: copied audio files when `-copy-artifacts=true`

The HTML report shows:

- voice: playable audio, raw Vosk transcript, normalized transcript, transcript reference, first deterministic outcome, final outcome after optional cleaner pass, and cleaner metadata
- text: raw input, normalized parser input, first deterministic outcome, final outcome after optional cleaner pass, and cleaner metadata
- scenario matrix: exact-rate per target state (`ready`, `needs_expiry`, `needs_name`, `reject`) for each variant
- difficulty matrix: exact-rate split by `medium` / `hard`
- tag matrix: exact-rate per family tag so regressions are visible by category instead of only by total score
- cleaner delta matrix and hardest failing tags for faster audit
- aggregate counters for `cleaner_eligible`, `cleaner_called`, `cleaner_changed_input`, `candidate_valid`, `cleaner_applied`, `cleaner_helped`, `cleaner_hurt`, `cleaner_noop`, and `cleaner_same_candidate`

Manual audit verdicts are stored in browser `localStorage` as `usable`, `partial`, or `bad` plus a free-form note.

## Audit

Every full benchmark run now expects a manual audit pass.

The audit should include:

- all failing cases
- all `cleaner_helped` and `cleaner_hurt` cases
- a spot-check sample of passes in each major scenario bucket
- suspicious cases where scorer strictness may not match product usefulness

The goal is not only to inspect model quality, but also to inspect benchmark quality:

- are there trivial cases left in the main score?
- are there bad oracles?
- is the scorer overly strict or too permissive?
- did a code change actually improve the product, or just overfit a narrow bucket?

## Notes

- `text` remains a parser benchmark. It is not mixed into acoustic or visual quality scoring.
- `voice` still depends on Vosk transcript quality; the cleaner stage only sees normalized transcript text, not the original audio.
- `first deterministic result` and `final card after cleaner` are now both measured explicitly, so fast-card quality and background-cleaner quality can be compared instead of collapsing everything into one score.
- `text` and `voice` are now intentionally kept at the same complexity tier. If a case becomes trivial after the pipeline matures, it should leave the main benchmark and move into unit/regression coverage.
- `Golos` source page: https://huggingface.co/datasets/bond005/sberdevices_golos_10h_crowd
