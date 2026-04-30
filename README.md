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
quartr auth login --api-key "$QUARTR_API_KEY"
quartr auth show
```

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

## Downloads

Download commands first retrieve metadata, then download the URL field from the response.

```bash
quartr reports download 12345 --output annual-report.pdf
quartr slides download 12345 --url-field fileUrl
```

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
