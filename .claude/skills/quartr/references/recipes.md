# quartr recipes

Worked examples for common workflows. Lift these patterns when the user
describes one of these intents.

Each recipe shows: the command(s), the relevant fields in the response, and
common pitfalls.

---

## 1. Find a company by ticker and grab its ID

**Intent:** "What's Apple's Quartr company ID?" / preflight before any
companyId-based query.

```bash
quartr companies list --tickers AAPL --fields id,name,country
```

Response shape (table):

```
id    name       country
4742  Apple Inc  US
```

For multiple tickers in one call:

```bash
quartr companies list --tickers AAPL,MSFT,NVDA --fields id,name,country
```

When parsing programmatically, prefer:

```bash
quartr companies list --tickers AAPL --format json
```

The `id` field is at `data[].id`.

---

## 2. Pull the last N earnings calls (or any events) for a ticker

**Intent:** "Show me Apple's recent earnings."

```bash
quartr events list --tickers AAPL --sort-by date --direction desc --limit 10 \
  --fields id,title,date,typeId
```

To restrict to earnings calls only, filter by event type:

```bash
quartr events list --tickers AAPL --type-ids 26,27,28,29 \
  --sort-by date --direction desc --limit 10 \
  --fields id,title,date,fiscalYear,fiscalPeriod
```

Type IDs come from `quartr event-types list --format csv` — the earnings call
quarters are 26 (Q1), 27 (Q2), 28 (Q3), 29 (Q4); 35 (H1) and 36 (H2) for
half-year reporters.

For one specific event:

```bash
quartr events get 406161 --format json
```

Returns a single object at `data.{id, title, date, fiscalYear, fiscalPeriod,
typeId, companyId, language, …}`.

---

## 3. Download the latest annual report (10-K)

**Intent:** "Get Apple's most recent 10-K."

Three steps:

```bash
# 1. Get company ID (or use --tickers directly in step 2 if the API accepts it)
COMPANY_ID=$(quartr companies list --tickers AAPL --format json \
  | jq -r '.data[0].id')

# 2. Find the latest 10-K (document type id 11)
quartr reports list --tickers AAPL --type-ids 11 \
  --sort-by date --direction desc --limit 1 \
  --fields id,fileUrl,eventId,createdAt
```

The `reports list` endpoint may not accept `--sort-by` (it's events-only); if
so, list and pick the highest-`createdAt`:

```bash
quartr reports list --tickers AAPL --type-ids 11 --limit 5 --format json \
  | jq '.data | sort_by(.createdAt) | reverse | .[0]'
```

```bash
# 3. Download
quartr reports download <id> --output apple-10k.pdf
```

Document type IDs (from `quartr document-types list --format csv`):

| ID | Form  | Name              |
|----|-------|-------------------|
| 11 | 10-K  | Annual report     |
| 13 | 20-F  | Annual report     |
| 18 | 40-F  | Annual report     |
|  7 | 10-Q  | Quarterly report  |
| 10 | 8-K   | Earnings release  |
| 14 | 6-K   | Earnings release  |
| 25 |       | Shareholder letter|
| 39 | DEF 14A | Proxy statement |

If the user doesn't specify the form, default to 10-K (id 11) for US-domiciled
issuers and 20-F (id 13) for foreign private issuers — check the company's
country first.

**Pitfall:** `--with-api-key` is *not* needed for the file download — Quartr's
`fileUrl` is publicly fetchable. Only add it if the file URL itself returns
401/403.

---

## 4. Fetch all transcripts for a ticker, paginated, with parent event metadata

**Intent:** "Get every Apple transcript ever, with the event each belongs to."

```bash
quartr transcripts list --tickers AAPL --expand event --all --format json \
  > apple-transcripts.json
```

`--all` follows `pagination.nextCursor` and auto-bumps `--limit` to 500.
`--expand event` merges the parent event object into each transcript row.

Resulting shape:

```json
{
  "data": [
    {
      "id": 1563891,
      "companyId": 4742,
      "eventId": 1597,
      "fileUrl": "https://files.quartr.com/raw-transcripts/…",
      "typeId": 15,
      "event": {
        "title": "Q4 2019",
        "date": "2019-10-30T13:04:00.000Z",
        "fiscalYear": 2019,
        "fiscalPeriod": "Q4",
        "typeId": 29,
        "language": "en"
      }
    }
  ],
  "count": 24,
  "pagination": {"nextCursor": null}
}
```

To download all of them, iterate with `jq`:

```bash
quartr transcripts list --tickers AAPL --all --format json \
  | jq -r '.data[].id' \
  | while read id; do
      quartr transcripts download "$id" --output "transcripts/$id.json"
    done
```

---

## 5. Stream a live earnings transcript

**Intent:** "Tail this live call as it runs."

```bash
# Find live transcripts in progress
quartr live transcripts list --states live,willBeLive

# Stream one
quartr live transcripts stream <id> --transcript-version 1.7
```

Output is JSONL (one JSON object per line). `--transcript-version` defaults to
1.7. If the user's API tier returns `403 Forbidden` on the list call, surface
the error — live endpoints are tier-gated.

---

## 6. Get an event summary (when tier permits)

**Intent:** "Summarize Apple's Q1 2026 call."

```bash
quartr events summary 406161 --length long --plain
```

`--length`: `line` | `short` | `long`. `--plain` strips embedded document
sources for cleaner reading.

If this returns `403 Forbidden`, the API tier doesn't include AI summaries —
report that to the user; do not retry or substitute a different endpoint.

The same `summary` operation exists on `reports`, `slides`, `transcripts`:

```bash
quartr reports summary <id> --length short --plain
quartr transcripts summary <id> --length long
```

---

## 7. Get transcript chapters

**Intent:** "What were the topic sections in this call?"

```bash
quartr transcripts chapters <transcript-id> --levels 1 --limit 50 \
  --format json
```

`--levels` filters to a hierarchy level. Level 1 is top-level chapters; higher
levels are sub-sections. The `chapters` operation also exists on `reports`,
`slides`, and `audio`.

---

## 8. Use the raw `request get` for an unwrapped endpoint

**Intent:** Hit an API path or query parameter the CLI doesn't model directly.

```bash
quartr request get /events --query tickers=AAPL --query limit=3 --format json
```

`--query` is repeatable. Use `--paginate` to follow `pagination.nextCursor`:

```bash
quartr request get /documents/transcripts \
  --query tickers=AAPL --query expand=event --paginate --format json
```

This is the right tool when:

- Quartr adds a new endpoint before the CLI supports it
- A query parameter exists in the API but isn't yet a CLI flag
- Debugging — passing the exact API params to verify behavior

---

## 9. Map filing forms (10-K, 8-K, etc.) to type IDs

**Intent:** User says "get the 8-K", we need a `--type-ids` value.

```bash
quartr document-types list --format csv | grep -i "8-k"
# 10,Earnings release,Report,8-K,1,…
```

Use `id=10`. Same lookup works for any form (`10-K`, `10-Q`, `20-F`, `DEF 14A`,
`S-1`, etc.).

For event types (Q1/Q2/Q3/Q4 earnings calls, AGM, Investor Day):

```bash
quartr event-types list --format csv
```

---

## 10. Empty results vs errors — how to tell the user

| Output                                    | Meaning                                              | What to say |
|-------------------------------------------|------------------------------------------------------|-------------|
| `No rows`                                 | Valid API response with empty `data` array           | "No matches for those filters." |
| `quartr api error: 403 Forbidden: …`      | API tier doesn't include this endpoint               | Quote the error; suggest narrowing scope or contacting Quartr to upgrade. |
| `quartr api error: 400 Bad Request: …`    | Bad parameter (e.g. unsupported `expand` value)      | Check the message body — it usually names the offending field. |
| `quartr api error: 404 Not Found: …`      | ID doesn't exist                                     | Verify the ID via a list query first. |
| `missing API key; set QUARTR_API_KEY …`   | No reachable credential                              | Ask user to set `QUARTR_API_KEY` or run `quartr auth login`. |

Don't retry 403/400/404 — they're not transient. The CLI already handles
429/5xx with backoff.
