package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// companyLookupLimit is the page size used when resolving tickers to
// companies. A ticker matches a handful of companies at most.
const companyLookupLimit = "500"

// tickerSpec is one entry of --tickers. Quartr matches a ticker string across
// every exchange it knows, so "CE" returns both Celanese and Credito
// Emiliano. An entry may be qualified with the exchange it has to come from:
// "NYSE:BLD" instead of "BLD".
type tickerSpec struct {
	exchange string
	ticker   string
}

func (s tickerSpec) String() string {
	if s.exchange == "" {
		return s.ticker
	}
	return s.exchange + ":" + s.ticker
}

// tickerEntries reads a company's ticker list, which Quartr shapes as
// [{"exchange": "NYSE", "ticker": "BLD"}, ...].
func tickerEntries(company map[string]any) []map[string]any {
	raw, ok := company["tickers"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// parseTickerSpecs splits a --tickers value into specs, dropping duplicates
// that differ only in case.
func parseTickerSpecs(s string) []tickerSpec {
	seen := map[string]bool{}
	var specs []tickerSpec
	for _, raw := range parseCSV(s) {
		spec := tickerSpec{ticker: raw}
		if exchange, ticker, ok := strings.Cut(raw, ":"); ok {
			spec = tickerSpec{exchange: strings.TrimSpace(exchange), ticker: strings.TrimSpace(ticker)}
		}
		if spec.ticker == "" {
			continue
		}
		key := strings.ToUpper(spec.String())
		if seen[key] {
			continue
		}
		seen[key] = true
		specs = append(specs, spec)
	}
	return specs
}

func anyQualified(specs []tickerSpec) bool {
	for _, s := range specs {
		if s.exchange != "" {
			return true
		}
	}
	return false
}

// bareTickerCSV is the value to send to the API: exchange qualifiers are a
// CLI concept, Quartr only understands the ticker string.
func bareTickerCSV(specs []tickerSpec) string {
	seen := map[string]bool{}
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		key := strings.ToUpper(s.ticker)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s.ticker)
	}
	return strings.Join(out, ",")
}

// matchedTickers renders the "EXCHANGE:TICKER" pairs on company that satisfy
// any of specs. With no specs it renders every pair the company has.
func matchedTickers(company map[string]any, specs []tickerSpec) []string {
	var hits []string
	seen := map[string]bool{}
	for _, entry := range tickerEntries(company) {
		exchange, ticker := idKey(entry["exchange"]), idKey(entry["ticker"])
		if ticker == "" {
			continue
		}
		if len(specs) > 0 && !matchesAny(specs, exchange, ticker) {
			continue
		}
		pair := ticker
		if exchange != "" {
			pair = exchange + ":" + ticker
		}
		if seen[pair] {
			continue
		}
		seen[pair] = true
		hits = append(hits, pair)
	}
	return hits
}

func matchesAny(specs []tickerSpec, exchange, ticker string) bool {
	for _, s := range specs {
		if !strings.EqualFold(s.ticker, ticker) {
			continue
		}
		if s.exchange == "" || strings.EqualFold(s.exchange, exchange) {
			return true
		}
	}
	return false
}

// lookupCompanies fetches company records by ticker or by CIK and annotates
// each with the "EXCHANGE:TICKER" pairs that matched, which is the field a
// human needs to tell two same-ticker companies apart.
func (a *app) lookupCompanies(ctx context.Context, param, value string, specs []tickerSpec) ([]map[string]any, error) {
	params := url.Values{}
	params.Set(param, value)
	params.Set("limit", companyLookupLimit)
	obj, _, err := a.client.GetJSON(ctx, "/companies", params)
	if err != nil {
		return nil, err
	}

	matched := make([]map[string]any, 0, len(dataRows(obj)))
	for _, company := range dataRows(obj) {
		hits := matchedTickers(company, specs)
		if len(specs) > 0 && len(hits) == 0 {
			continue
		}
		company["matchedTickers"] = strings.Join(hits, ",")
		matched = append(matched, company)
	}
	return matched, nil
}

// resolveQualifiedTickers turns exchange-qualified --tickers into an explicit
// companyIds filter. Filtering the returned page client-side would be wrong:
// the rows the wrong company occupies still count against --limit, so the
// company you asked for can be pushed off the page entirely. Resolving first
// and filtering server-side by id is the recipe the issue describes, done in
// one command instead of two.
func (a *app) resolveQualifiedTickers(ctx context.Context, specs []tickerSpec) ([]string, error) {
	companies, err := a.lookupCompanies(ctx, "tickers", bareTickerCSV(specs), specs)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", specList(specs), err)
	}
	ids := make([]string, 0, len(companies))
	for _, company := range companies {
		if id := idKey(company["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, usagef("no company matches %s; run `quartr companies resolve %s` to see the candidates",
			specList(specs), specs[0].ticker)
	}
	return ids, nil
}

// applyQualifiedTickers rewrites --tickers into an explicit companyIds filter
// when any entry names an exchange, and collapses case-duplicate tickers
// either way.
func (a *app) applyQualifiedTickers(ctx context.Context, r resource, lf *listFlags) error {
	specs := parseTickerSpecs(lf.tickers)
	if len(specs) == 0 {
		return nil
	}
	if !anyQualified(specs) {
		lf.tickers = bareTickerCSV(specs)
		return nil
	}
	if !r.listParams.allows("tickers") {
		return usagef("--tickers is not supported by `quartr %s list`", r.name)
	}

	ids, err := a.resolveQualifiedTickers(ctx, specs)
	if err != nil {
		return err
	}
	lf.tickers = ""
	lf.companyIDs = strings.Join(append(parseCSV(lf.companyIDs), ids...), ",")
	return nil
}

func specList(specs []tickerSpec) string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.String())
	}
	return strings.Join(out, ",")
}

// looksLikeCIK reports whether a `companies resolve` argument should be
// treated as a SEC CIK rather than a ticker.
func looksLikeCIK(s string) bool {
	if len(s) < 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// dedupeCSV removes repeated entries from a comma-separated filter value,
// ignoring case. Quartr accepts duplicates, but they inflate the URL and make
// `--tickers "$LIST"` fragile when the caller builds the list by hand.
func dedupeCSV(s string) string {
	seen := map[string]bool{}
	out := make([]string, 0, 8)
	for _, v := range parseCSV(s) {
		key := strings.ToUpper(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return strings.Join(out, ",")
}
