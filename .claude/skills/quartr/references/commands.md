# quartr commands reference

Full command/flag/path reference. Pull this in when SKILL.md doesn't cover the
specific flag or endpoint at hand.

## Resource → operations → API path

| Command            | Operations                                  | Base path                  | Notes                          |
|--------------------|---------------------------------------------|----------------------------|--------------------------------|
| `auth`             | `login`, `show`, `logout`                   | (local)                    | Manages `~/.config/quartr/config.json` |
| `companies`        | `list`, `get`                               | `/companies`               | Uses `ids` API param, not `companyIds` |
| `events`           | `list`, `get`, `summary`                    | `/events`                  | `summary` is tier-restricted   |
| `documents`        | `list`, `get`, `download`                   | `/documents`               | Generic parent; prefer typed resources |
| `reports`          | `list`, `get`, `summary`, `pages`, `download` | `/documents/reports`     | `fileUrl` is the download field |
| `slides`           | `list`, `get`, `summary`, `pages`, `download` | `/documents/slides`      | `fileUrl` is the download field |
| `transcripts`      | `list`, `get`, `summary`, `chapters`, `download` | `/documents/transcripts` | `fileUrl` is the download field |
| `audio`            | `list`, `get`, `chapters`, `download`       | `/audio`                   | `list` may be tier-restricted; `fileUrl` |
| `live`             | `list`, `get`                               | `/live`                    | Honors `transcriptVersion`     |
| `live audio`       | `list`, `get`, `download`                   | `/live/audio`              | Download field is `audio`      |
| `live transcripts` | `list`, `get`, `stream`                     | `/live/transcripts`        | `list` may be tier-restricted; stream field is `transcript` |
| `event-types`      | `list`, `get`                               | `/event-types`             | Lookup table                   |
| `document-types`   | `list`, `get`                               | `/document-types`          | Lookup table                   |
| `request`          | `get`                                       | (any path)                 | Escape hatch; `--query k=v --paginate` |

`live audio` and `live transcripts` accept either `quartr live audio …` or
`quartr live-audio …` — the dispatcher normalizes both.

## Global flags

| Flag             | Env var          | Default                                | Notes                          |
|------------------|------------------|----------------------------------------|--------------------------------|
| `--api-key`      | `QUARTR_API_KEY` | (none)                                 | Required for all API calls     |
| `--base-url`     | `QUARTR_BASE_URL`| `https://api.quartr.com/public/v3`     |                                |
| `--config`       | `QUARTR_CONFIG`  | `~/.config/quartr/config.json`         | File is mode 0600              |
| `--format`       | `QUARTR_FORMAT`  | `table`                                | `table` / `json` / `csv` / `raw` |
| `--timeout`      | `QUARTR_TIMEOUT` | `30s`                                  | Go duration                    |
| `--no-config`    | —                | false                                  | Skip the config file entirely  |
| `--debug`        | —                | false                                  | Print GET URL to stderr        |
| `--help`, `-h`   | —                | —                                      | Print help and exit            |
| `--version`      | —                | —                                      | Print version and exit         |

Auth precedence: flags > env > config file > defaults.

## List flags (per-resource compatibility)

```
--limit N                  page size, 1..500, default 10
--cursor STRING            pagination cursor
--direction asc|desc       sort direction
--all                      follow pagination.nextCursor (auto-bumps limit to 500 if not set)
--fields a,b,c             output columns; supports dotted paths

--tickers AAPL,MSFT        comma-separated tickers
--company-ids 4742         maps to "ids" for companies, "companyIds" elsewhere
--countries US,GB          ISO country codes
--exchanges NYSE,NASDAQ    exchange symbols
--isins US0378331005       ISINs
--ciks 0000320193          SEC CIKs
--ids foo,bar              alias used by companies-only consumers
--start-date 2024-01-01    ISO 8601
--end-date 2024-12-31      ISO 8601
--updated-after 2024-01-01 incremental sync lower bound
--updated-before 2024-12-31
--expand event             merge related objects into response
--type-ids 1,2,3           events / documents* / transcripts* / reports* / slides* / audio*
--event-ids 128301         documents* / transcripts* / reports* / slides* / audio* / live*
--document-group-ids foo   documents* / transcripts* / reports* / slides*
--states live,willBeLive   live, live-transcripts, live-audio
--transcript-version 1.7   live, live-transcripts, transcripts (get only), audio (get only)
--sort-by date             events list only
--levels 1,2               chapters subcommand on reports/slides/transcripts/audio
```

Filter flags that aren't allowed for a resource are silently dropped; the
allowed set is enforced by `paramSet` in `internal/cli/resources.go`.

## Per-operation flags beyond list

| Operation        | Flag                     | Notes                                              |
|------------------|--------------------------|----------------------------------------------------|
| `<r> get`        | `--fields`               | Comma-separated, supports dotted paths             |
| `<r> get`        | `--expand`               | Where supported (documents/reports/slides/transcripts/audio) |
| `<r> get`        | `--transcript-version`   | live, live-transcripts                             |
| `<r> summary`    | `--length line\|short\|long` | events, reports, slides, transcripts          |
| `<r> summary`    | `--plain`                | Strip embedded document sources                    |
| `<r> summary`    | `--fields`               | Output columns                                     |
| `<r> pages`      | list flags               | reports, slides only                               |
| `<r> chapters`   | list flags + `--levels`  | transcripts, audio only                            |
| `<r> download`   | `--output PATH`          | Defaults to `<resource>-<id>.<ext>` in cwd         |
| `<r> download`   | `--url-field NAME`       | Defaults to `fileUrl` (or resource-specific)       |
| `<r> download`   | `--with-api-key`         | Send `x-api-key` when fetching the file URL        |
| `<r> download`   | `--expand`               | On the metadata request                            |
| `<r> stream`     | `--transcript-version`   | Default `1.7`                                      |
| `<r> stream`     | `--with-api-key`         | Send key when opening the stream URL               |
| `request get`    | `--query k=v` (repeatable) | Multiple `--query` flags allowed                 |
| `request get`    | `--paginate`             | Follow `pagination.nextCursor`                     |
| `request get`    | `--fields`               | Output columns                                     |

## Output formats

Implemented in `internal/output/output.go`.

- **`table`** (default)
  - Detects single-object vs list-of-objects responses
  - Auto-selects useful columns when `--fields` is not provided
  - Truncates each cell value at ~120 characters
  - Prints `No rows` for empty list responses
- **`json`**
  - Pretty-printed with 2-space indent
  - Numbers preserved as `json.Number` to avoid float coercion
- **`csv`**
  - RFC 4180; header row from `--fields` or auto-selected columns
  - Each row escaped according to spec
- **`raw`**
  - Plain bytes/string, no parsing or formatting

`--fields` paths support dotted access:

```
--fields id,event.title,event.fiscalYear,data.fileUrl
```

Missing fields render as empty cells in `table`/`csv` and as `null` in `json`.

## Filename behavior on download

When `--output` is omitted, downloads are saved to:

```
<resource>-<id><ext>
```

with `<ext>` taken from the URL path (`.pdf`, `.json`, `.mp3`, …) or `.bin` if
not detectable. Non-alphanumeric characters in `<resource>-<id>` are replaced
with `-`.

## Retry behavior

- Automatic retry on `429` and `5xx`
- Honors `Retry-After` header when present
- Up to 3 attempts with exponential backoff (250ms, 500ms)
- Errors surface as `quartr api error: <status>: <body>` on final failure

## Lookup tables (run these once, then reference in flags)

```bash
quartr event-types list --format csv
# id,name,parent
# 26,Q1,Earnings call
# 27,Q2,Earnings call
# 28,Q3,Earnings call
# 29,Q4,Earnings call
# 35,H1,Earnings call
# 36,H2,Earnings call
#  4,AGM,Annual General Meeting
#  2,CMD,Capital Markets Day
# 31,Investor Day
# 33,Analyst Day
# …

quartr document-types list --format csv
# id,name,form
# 11,Annual report,10-K
# 13,Annual report,20-F
# 18,Annual report,40-F
#  7,Quarterly report,10-Q
# 10,Earnings release,8-K
# 14,Earnings release,6-K
# 15,Transcript,
#  5,Slides,
# 25,Shareholder letter,
# 39,Proxy statement,DEF 14A
# 27,Registration statement,S-1
# …
```

When the user names a filing form (10-K, 8-K, etc.), look up the `id` first
and pass it via `--type-ids`.

## Key code locations (for skill maintenance)

If the CLI gets new commands, refresh this reference from:

- `internal/cli/app.go` — top-level command dispatch
- `internal/cli/resources.go` — resource map, paths, allowed param sets
- `internal/cli/flags.go` — global flags, listFlags, `toParams`
- `internal/cli/handlers.go` — list/get/summary/pages/chapters/download/stream/request
- `internal/output/output.go` — format implementations, dotted-path lookup
- `internal/quartr/client.go` — retry, backoff, BuildURL
- `internal/quartr/config.go` — config file precedence and shape
