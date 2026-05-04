# Voice Cleaner Tuning

This repo now uses a cleaner-only LLM path:

- fast parser creates the first draft
- `cleaner` receives normalized text only
- local deterministic code rebuilds the candidate draft
- the draft is updated only if the local diff says the cleaned candidate is strictly better

If we want a small Gemma model to help more on real Russian voice input, prompt tuning alone is not enough. The next useful step is a small supervised dataset of real failures.

## What To Collect

Collect `20-50` real voice cases that currently fail or look noisy in the bot.

Prioritize these buckets:

- leading filler: `по фарш послезавтра`, `так у меня один банан до завтра`
- trailing filler: `хлеб через неделю пожалуйста`, `йогурт завтра на дом`
- quantity noise: `ложкарев полкилограмма пельмени`
- brand cleanup: `липтон зеленый чай закажи пожалуйста`
- date noise: `молоко тут двадцать девятого`, `фарш до второго число`
- intent clutter: `так мне нужно записать молоко до двадцать шестого`

Avoid trivial cases like one clean word with one clean date.

## How To Label

For each case, write:

- `raw_input`: what the user said or what Vosk produced
- `normalized_input`: current normalized transcript/text from the bot
- `cleaned_input`: the ideal cleaner output
- `want_state`: `ready`, `needs_expiry`, `needs_name`, or `reject`
- `want_name`
- `want_raw_deadline_phrase`
- `note`

Rules for `cleaned_input`:

- keep only one grocery item
- keep only a real date phrase if one exists
- remove filler, wake words, request verbs, quantity/package chatter, store/delivery chatter
- do not invent brands or dates
- keep Russian wording in Russian

## File Format

Use JSONL and append one JSON object per line.

Template:

```json
{"id":"voice-tune-001","source_kind":"voice","raw_input":"по фарш послезавтра","normalized_input":"по фарш послезавтра","cleaned_input":"фарш послезавтра","want_state":"ready","want_name":"фарш","want_raw_deadline_phrase":"послезавтра","note":"leading filler"}
```

Seed file:

- [`review_cleaner_seed.jsonl`](../../internal/ingest/testdata/review_tuning/review_cleaner_seed.jsonl)

## What To Send Back

Once you collect the cases, put them into:

- `/Users/igor/projects/shelfy/internal/ingest/testdata/review_tuning/review_cleaner_local.jsonl`

Then the next pass is:

1. convert the JSONL into cleaner training pairs
2. run prompt eval on the same set first
3. if prompt-only is still weak, fine-tune Gemma on those cleaner pairs
4. rerun `live_hard` plus live dogfood cases
