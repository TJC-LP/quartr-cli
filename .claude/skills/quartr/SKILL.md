---
name: quartr
description: Query Quartr Public API v3 via the local `quartr` CLI — companies, events, earnings calls, transcripts, reports, slides, audio, live events. Use when the user asks about a ticker's earnings or fiscal periods, SEC filings (10-K / 10-Q / 8-K / 20-F / proxy) via Quartr, downloading transcripts or reports, streaming live calls, or anything sourced from api.quartr.com / quartr.com.
---

# quartr

Drive the `quartr` CLI (Quartr Public API v3) to fetch financial intelligence:
companies, events, transcripts, reports, slides, audio, and live events.

## Binary

This skill ships inside the `quartr-cli` repo. Build the binary from the repo
root:

```bash
go build -o bin/quartr ./cmd/quartr
```

Then invoke it as `./bin/quartr` from the repo root, or `$(git rev-parse --show-toplevel)/bin/quartr`
from anywhere inside the worktree. Use `go install ./cmd/quartr` instead to
install it on `$GOBIN/quartr` for PATH-style invocation.

## Auth

Precedence (highest to lowest):

1. `--api-key` flag
2. `QUARTR_API_KEY` env var
3. `~/.config/quartr/config.json` (written 0600 by `quartr auth login`)

For durable use, run `quartr auth login` once with `QUARTR_API_KEY` exported —
the command picks up the env var via the standard precedence and persists it
to `~/.config/quartr/config.json`. Avoid `--api-key VALUE` on `auth login`:
the literal key ends up in shell history, `ps`, scrollback, and CI logs.
When piping from a secret store, use `--api-key-stdin`:

```bash
# Idiomatic
export QUARTR_API_KEY=...
quartr auth login

# Pipe from a secret store
op read op://Personal/Quartr/api_key | quartr auth login --api-key-stdin
```

For project-scoped use with a `.env` file:

```bash
set -a && source .env && set +a && quartr companies list --tickers AAPL
```

If no key is reachable, the CLI errors with a clear message — do not try to
guess one.

## Output formats

- `--format json` — pretty-printed, use whenever Claude needs to parse output
- `--format table` (default) — tab-delimited, good for showing the user
- `--format csv` — RFC 4180, header row, narrow columns
- `--format raw` — raw bytes, useful for piping
- `--fields a,b,c` — pick columns; supports dotted paths like
  `event.title`, `event.fiscalYear`, `data.fileUrl`

When parsing programmatically, always pair `--format json` with the appropriate
filter flags. Default `table` truncates long values at ~120 chars.

## Pagination

```bash
quartr transcripts list --tickers AAPL --all --format json
```

`--all` follows `pagination.nextCursor` until exhausted. If `--limit` is not
explicitly passed, `--all` raises it to 500 to minimize round-trips.

## Quick command card

```bash
# Companies
quartr companies list --tickers AAPL,MSFT --fields id,name,country
quartr companies resolve CE                # every company using that ticker
quartr companies get 4742 --format json

# Events (earnings calls, AGMs, etc.)
quartr events list --tickers AAPL --sort-by date --direction desc --limit 10
quartr events list --tickers NYSE:BLD --expand company --limit 5
quartr events get 406161 --format json

# Transcripts
quartr transcripts list --tickers AAPL --expand event,company --limit 10
quartr transcripts get <id> --format json
quartr transcripts download <id> --output transcript.json
quartr transcripts download <id> --output - | jq .    # stream, no file
quartr transcripts chapters <id> --levels 1,2

# Reports (10-K, 10-Q, 8-K, etc.)
quartr reports list --tickers AAPL --type-ids 11 --limit 5
quartr reports download <id> --output annual-report.pdf
quartr reports pages <id> --format csv
quartr reports summary <id> --length long --plain

# Slides
quartr slides list --tickers AAPL --limit 5
quartr slides download <id>
quartr slides pages <id>

# Audio
quartr audio list --tickers AAPL --limit 5
quartr audio download <id> --output earnings.mp3

# Live events
quartr live list --states live,willBeLive
quartr live transcripts list --states live
quartr live transcripts stream <id> --transcript-version 1.7

# Lookup tables (whole catalog, not a first page; map names → IDs before filtering)
quartr event-types list --format csv       # Q1=26, Q2=27, Q3=28, Q4=29 …
quartr document-types list --format csv    # 10-K=11, 10-Q=7, 8-K=10, 25=shareholder letter …
```

## Escape hatch

For endpoints or query params not yet wrapped:

```bash
quartr request get /any/path --query key=value --query k2=v2 [--paginate]
quartr request get /events --query tickers=AAPL --query limit=3 --format json
```

`--paginate` follows `pagination.nextCursor` like `--all`.

## Quirks

- **Companies use `ids`, not `companyIds`.** The CLI auto-maps `--company-ids`
  to `ids` for the `companies` resource. Other resources use `companyIds`.
  This only matters when reading raw API responses or using `request get`.
- **Only `events list` can sort.** `--sort-by` (fields `id`, `date`) exists
  nowhere else and is rejected with exit 2, because those endpoints return rows
  in insertion order — the newest filing or call is often *not* on the first
  page. To get the latest documents: sort events, then filter by `--event-ids`,
  or pull with `--all` and sort locally on `createdAt`. `--direction` works
  everywhere but reverses insertion order, not date order.
- **Tickers collide across exchanges.** `--tickers CE` returns Celanese,
  Credito Emiliano and Cortus Energy. Run `quartr companies resolve <ticker>`
  when a symbol might be shared, then either use `--company-ids` or qualify the
  ticker as `NYSE:BLD` (the CLI resolves it to a companyId before querying).
  There is no name search in the API — tickers and CIKs only.
- **`--expand company` is a client-side join.** The API rejects
  `expand=company`; the CLI strips it and batch-fetches `/companies` instead.
  Use it whenever rows need to be attributable — otherwise they carry only a
  bare `companyId` and a collision is invisible.
- **Tier-restricted endpoints** return `403 Forbidden` on the user's API tier.
  Observed restrictions: `events summary`, `audio list`, `live transcripts list`.
  The CLI prints a `hint:` line clarifying that 403 is entitlement, not
  authentication. Surface the error verbatim — do not retry, hide, silently
  fall back, or start debugging the API key. A rejected key returns 401.
- **Downloads always write a file** named `<resource>-<id>.<ext>` unless
  `--output` says otherwise; the `Saved <path>` line goes to stderr. To pipe or
  redirect the document itself, use `--output -`.
- **Downloads don't send the API key by default.** The metadata response
  contains a public file URL. Add `--with-api-key` only if a 401/403 occurs
  fetching the file URL itself.
- **`--debug`** prints the full GET URL to stderr — useful when a query returns
  unexpected data.
- **Retries** on 429 / 5xx are automatic with backoff and `Retry-After`
  honoring; no need to wrap commands in retry logic.

## Common workflows

For a worked example of each, see `references/recipes.md`:

- Find a company by ticker and grab its ID
- Pull the last N earnings calls for a ticker
- Download the latest annual report (10-K)
- Fetch all transcripts for a ticker, paginated, with parent event metadata
- Stream a live earnings transcript
- Use the raw `request get` for an unwrapped endpoint

For the full command/flag/path matrix, see `references/commands.md`.
