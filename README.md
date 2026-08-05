# quartr-cli

A dependency-free Go CLI for the Quartr Public API v3.

It is designed for API subscribers who want a terminal-friendly interface for company, event, document, transcript, audio, and live-event workflows.

## Features

- Uses the official `x-api-key` header.
- Targets Quartr Public API v3 by default: `https://api.quartr.com/public/v3`.
- Stores local config at `~/.config/quartr/config.json` with file mode `0600`.
- Supports environment variables: `QUARTR_API_KEY`, `QUARTR_BASE_URL`, `QUARTR_FORMAT`, `QUARTR_TIMEOUT`, `QUARTR_CONFIG`.
- Supports `table`, `json`, `csv`, and `raw` output.
- Supports cursor pagination with `--all`.
- Supports downloads from metadata URL fields.
- Includes a raw `request get` escape hatch for endpoints or parameters not wrapped yet.
- Retries transient `429` and `5xx` responses with short backoff and honors `Retry-After` when present.
- Uses only the Go standard library.

## Build

```bash
go build -o bin/quartr ./cmd/quartr
```

Or install from the project directory:

```bash
go install ./cmd/quartr
```

## Configure

```bash
export QUARTR_API_KEY="your-api-key"
```

Or store it locally:

```bash
# Reads QUARTR_API_KEY from the environment if set; otherwise prompts.
quartr auth login
quartr auth show
```

The key is written to `~/.config/quartr/config.json` with file mode `0600`.

For piping the key in (e.g. from a secret store) without exposing it via argv:

```bash
op read op://Personal/Quartr/api_key | quartr auth login --api-key-stdin
```

`--api-key VALUE` is also supported but discouraged for `auth login` because
the value leaks via shell history, `ps`, terminal scrollback, and CI logs.

Config precedence is:

1. global CLI flags
2. environment variables
3. config file
4. built-in defaults

## Global flags

```text
--api-key KEY        Quartr API key; defaults to QUARTR_API_KEY or config
--base-url URL       API base URL
--config PATH        config path
--format FORMAT      table, json, csv, or raw
--timeout DURATION   HTTP timeout, e.g. 30s
--no-config          ignore config file
--debug              print GET URLs to stderr
--version            print version
-h, --help           show help
```

Global flags can appear before or after the command:

```bash
quartr companies list --tickers AAPL --format json
```

## Supported resources

```text
companies
events
documents
reports
slides
transcripts
audio
live
live audio
live transcripts
event-types
document-types
```

## Examples

List companies by ticker:

```bash
quartr companies list --tickers AAPL
```

Find every company sharing a ticker:

```bash
quartr companies resolve BLD
```

List recent Apple events:

```bash
quartr events list --tickers AAPL --sort-by date --direction desc --limit 5
```

Fetch one event as JSON:

```bash
quartr events get 128301 --format json
```

Fetch an event summary:

```bash
quartr events summary 128301 --length long --plain
```

List transcripts with expanded event metadata:

```bash
quartr transcripts list --tickers MSFT --expand event --limit 10
```

Fetch all transcript pages by cursor:

```bash
quartr transcripts list --tickers AAPL --all --format json
```

Download a transcript document:

```bash
quartr transcripts download 432907 --output transcript.json
```

List report pages:

```bash
quartr reports pages 12345 --format csv
```

Stream a live transcript JSONL URL to stdout:

```bash
quartr live transcripts stream 127537 --transcript-version 1.7
```

Use the raw request escape hatch:

```bash
quartr request get /events --query tickers=AAPL --query limit=3 --format json
quartr request get /documents/transcripts --query tickers=AAPL --query expand=event --paginate
```

## Output fields

For `table` and `csv`, pass `--fields` to pick columns, including dotted paths:

```bash
quartr events list --tickers AAPL --fields id,title,date,typeId,event.fiscalYear
```

Without `--fields`, the CLI chooses useful fields from the returned JSON.

## Endpoint notes

Most list commands support a shared set of filters where Quartr exposes them:

```text
--tickers
--company-ids
--countries
--exchanges
--isins
--ciks
--start-date
--end-date
--updated-after
--updated-before
--limit
--cursor
--direction
```

Endpoint-specific filters are also available where relevant:

```text
--type-ids
--event-ids
--document-group-ids
--expand
--states
--transcript-version
--sort-by
--levels
```

Companies are the one common exception where Quartr uses `ids` instead of `companyIds`. This CLI maps `--company-ids` to `ids` for `companies list`.

## Tickers and exchange collisions

Quartr matches a ticker string across every exchange it knows, so a US symbol quietly pulls in foreign namesakes:

```bash
quartr companies resolve CE
```

```text
id     name                     country  matchedTickers
5977   Celanese Corporation     US       NYSE:CE
16679  Credito Emiliano S.p.A.  IT       BIT:CE
16930  Cortus Energy            SE       OM:CE
```

`companies resolve` accepts a ticker, an exchange-qualified ticker, or a CIK, and prints every candidate with the exchange pairs that matched. Use it whenever a symbol might be shared. (CIKs deserve the same caution — `companies resolve 0001061630` returns Blackstone Mortgage Trust, which is rarely what the caller expected.)

Once you know the exchange, qualify the ticker and the CLI does the disambiguation for you:

```bash
quartr events list --tickers NYSE:BLD --limit 4
```

`EXCHANGE:TICKER` is resolved to a `companyId` before the real request goes out:

```text
GET /companies?limit=500&tickers=BLD      # resolve
GET /events?companyIds=11909&limit=4      # then query
```

Resolving up front rather than filtering the response matters: rows belonging to the other company would otherwise still count against `--limit`, so the company you asked for can be pushed off the page entirely.

Qualifiers are per entry, so `--tickers AAPL,NYSE:BLD` works; once any entry is qualified, all of them are resolved to ids. Duplicate tickers are collapsed case-insensitively.

The Quartr API has no company name search — there is no `search`, `query`, or `name` parameter on `/companies` — so `resolve` takes tickers and CIKs only.

## Expanding companies

Quartr has no server-side company expansion — `/events` rejects `expand` outright and the document endpoints accept only `expand=event`. Pass `--expand company` anyway and the CLI performs the join itself, batching the distinct `companyId` values into `/companies` calls of up to 100 ids:

```bash
quartr events list --tickers ACA --limit 6 --expand company \
  --fields id,date,title,companyId,company.name,company.country
```

```text
id     date                      title              companyId  company.name          company.country
243    2021-08-05T00:00:00.000Z  Q2 2021            3694       Arcosa Inc            US
22156  2022-05-05T16:50:34.000Z  Q1 2022            12301      Crédit Agricole S.A.  FR
```

`company.name` and `company.country` are picked up automatically when `--fields` is omitted, which is usually how you notice that one ticker matched two companies on different exchanges.

`--expand event,company` works: `event` goes to the API, `company` is joined locally. The join also applies to `get`, runs once across all pages under `--all`, and is skipped for rows that already carry a `company` object. If the `/companies` lookup fails, the rows are still printed and a warning goes to stderr.

## Sorting

`--sort-by` is only implemented by `/events`, where the accepted fields are `id` and `date`. Every other list endpoint rejects the parameter outright, so the CLI now fails with exit code 2 instead of dropping the flag:

```bash
quartr transcripts list --tickers AAPL --sort-by date
# --sort-by is not supported by `quartr transcripts list` ... (exit 2)
```

This matters because the failure used to be invisible: document endpoints return rows in insertion order, so the newest filings and calls are simply absent from the first page. Sort events first, then fetch documents by event id:

```bash
quartr events list --tickers AAPL --sort-by date --direction desc --limit 5
quartr transcripts list --event-ids 406161
```

`--direction asc|desc` is accepted by every list endpoint, but it reverses insertion order, not date order.

## Type ids

`--type-ids` takes the ids from Quartr's own lookup tables. Both are returned whole — they are bounded catalogs (46 document types, 34 event types) and the default page size used to cut them off at 10, which is why ids like 25 and 46 looked undocumented:

```bash
quartr document-types list --format csv
quartr event-types list --format csv
```

The ones you will reach for most, as of this writing:

| Document type | id | Event type | id |
|---|---|---|---|
| Annual report (10-K) | 11 | Q1 earnings call | 26 |
| Quarterly report (10-Q) | 7 | Q2 earnings call | 27 |
| Earnings release (8-K) | 10 | Q3 earnings call | 28 |
| Annual report (20-F) | 13 | Q4 earnings call | 29 |
| Slides | 5 | H1 / H2 earnings call | 35 / 36 |
| Transcript | 15 | Capital Markets Day | 2 |
| Shareholder letter | 25 | Annual General Meeting | 4 |
| Proxy statement (DEF 14A) | 39 | Investor Day | 31 |
| Proxy statement (DEFM14A) | 46 | Guidance / update | 8 |

Treat the table as a convenience: Quartr adds types over time, so the lookup commands above are the source of truth.

## Tier-restricted endpoints

Some endpoints are gated by API plan and answer with a bare `403 Forbidden`, which reads exactly like a credentials failure — especially since everything else keeps working with the same key. The CLI now says which one it is:

```text
quartr api error: 403 Forbidden: {"message":"Forbidden","statusCode":403}
hint: 403 means this endpoint is not included in your API tier, not that your key is wrong
(a rejected key returns 401). ...
```

Endpoints observed gated this way: `events summary`, `audio list`, `live transcripts list`. A rejected key returns `401` and gets a hint pointing at `quartr auth show` instead.

## Downloads

Download commands first retrieve metadata, then download the URL field from the response.

```bash
quartr reports download 12345 --output annual-report.pdf
quartr slides download 12345 --url-field fileUrl
```

A download always writes a file unless you ask for stdout. `--output -` streams the document itself:

```bash
quartr transcripts download 432907 --output - > transcript.json
quartr transcripts download 432907 --output - | jq '.transcript.text'
```

Without `--output`, the file is named after the resource and id (`transcripts-432907.json`) in the current directory. The `Saved <path>` confirmation goes to **stderr**, so a plain `> file` redirect never captures it.

By default, the CLI does not include `x-api-key` when fetching a returned file URL. Add `--with-api-key` if your URL requires it:

```bash
quartr transcripts download 432907 --with-api-key
```

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build -o bin/quartr ./cmd/quartr
```
