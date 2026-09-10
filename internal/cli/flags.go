package cli

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

func extractGlobalFlags(args []string) (globalOverrides, []string, error) {
	var g globalOverrides
	stripped := make([]string, 0, len(args))
	boolFlags := map[string]func(){
		"no-config": func() { g.NoConfig = true },
		"debug":     func() { g.Debug = true },
		"help":      func() { g.Help = true },
		"version":   func() { g.Version = true },
	}
	stringFlags := map[string]func(string){
		"api-key":  func(v string) { g.APIKey = &v },
		"base-url": func(v string) { g.BaseURL = &v },
		"config":   func(v string) { g.Config = &v },
		"format":   func(v string) { g.Format = &v },
		"timeout":  func(v string) { g.Timeout = &v },
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			stripped = append(stripped, args[i+1:]...)
			break
		}
		if a == "-h" {
			g.Help = true
			continue
		}
		if !strings.HasPrefix(a, "--") || a == "--" {
			stripped = append(stripped, a)
			continue
		}

		nameVal := strings.TrimPrefix(a, "--")
		name, val, hasEq := nameVal, "", false
		if k, v, found := strings.Cut(nameVal, "="); found {
			name, val, hasEq = k, v, true
		}
		if f, ok := boolFlags[name]; ok {
			f()
			continue
		}
		if f, ok := stringFlags[name]; ok {
			if !hasEq {
				if i+1 >= len(args) {
					return g, nil, fmt.Errorf("--%s requires a value", name)
				}
				i++
				val = args[i]
			}
			f(val)
			continue
		}
		stripped = append(stripped, a)
	}

	g.RawArgs = args
	g.Stripped = stripped
	return g, stripped, nil
}

type listFlags struct {
	limit             int
	cursor            string
	direction         string
	all               bool
	fields            string
	countries         string
	exchanges         string
	tickers           string
	isins             string
	ciks              string
	openfigis         string
	companyIDs        string
	ids               string
	startDate         string
	endDate           string
	updatedAfter      string
	updatedBefore     string
	typeIDs           string
	eventIDs          string
	documentGroupIDs  string
	expand            string
	states            string
	transcriptVersion string
	sortBy            string
	levels            string
}

func addListFlags(fs *flag.FlagSet, lf *listFlags) {
	fs.IntVar(&lf.limit, "limit", 10, "items per page; Quartr supports 1..500")
	fs.StringVar(&lf.cursor, "cursor", "", "pagination cursor")
	fs.StringVar(&lf.direction, "direction", "", "sort direction: asc or desc")
	fs.BoolVar(&lf.all, "all", false, "fetch all pages by following pagination.nextCursor")
	fs.StringVar(&lf.fields, "fields", "", "comma-separated output fields")
	fs.StringVar(&lf.countries, "countries", "", "comma-separated ISO country codes")
	fs.StringVar(&lf.exchanges, "exchanges", "", "comma-separated exchange symbols")
	fs.StringVar(&lf.tickers, "tickers", "", "comma-separated tickers; qualify with an exchange to avoid collisions, e.g. AAPL,NYSE:BLD")
	fs.StringVar(&lf.isins, "isins", "", "comma-separated ISINs")
	fs.StringVar(&lf.ciks, "ciks", "", "comma-separated SEC CIKs")
	fs.StringVar(&lf.openfigis, "openfigis", "", "comma-separated OpenFIGI codes (figi, compositeFigi, or shareClassFigi); companies only")
	fs.StringVar(&lf.companyIDs, "company-ids", "", "comma-separated Quartr company IDs")
	fs.StringVar(&lf.ids, "ids", "", "comma-separated IDs for resources that support ids")
	fs.StringVar(&lf.startDate, "start-date", "", "ISO 8601 start date")
	fs.StringVar(&lf.endDate, "end-date", "", "ISO 8601 end date")
	fs.StringVar(&lf.updatedAfter, "updated-after", "", "ISO 8601 updatedAfter")
	fs.StringVar(&lf.updatedBefore, "updated-before", "", "ISO 8601 updatedBefore")
	fs.StringVar(&lf.typeIDs, "type-ids", "", "comma-separated type IDs")
	fs.StringVar(&lf.eventIDs, "event-ids", "", "comma-separated event IDs")
	fs.StringVar(&lf.documentGroupIDs, "document-group-ids", "", "comma-separated document group IDs")
	fs.StringVar(&lf.expand, "expand", "", "comma-separated fields to expand: event (API) or company (joined client-side)")
	fs.StringVar(&lf.states, "states", "", "comma-separated live states")
	fs.StringVar(&lf.transcriptVersion, "transcript-version", "", "live transcript stream version, e.g. 1.7")
	fs.StringVar(&lf.sortBy, "sort-by", "", "sort field; only `events list` supports it (id, date)")
	fs.StringVar(&lf.levels, "levels", "", "comma-separated chapter levels")
}

// listValuedParams are the query parameters Quartr reads as comma-separated
// lists, and so the ones worth deduplicating. Scalars are left alone —
// a cursor is an opaque token that may legitimately contain a comma.
var listValuedParams = params(
	"countries", "exchanges", "tickers", "isins", "ciks", "openfigis", "companyIds", "ids",
	"typeIds", "eventIds", "documentGroupIds", "states", "levels", "expand",
)

func (lf listFlags) toParams(allowed paramSet, companyEndpoint bool) url.Values {
	p := url.Values{}
	add := func(apiName, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		if !allowed.allows(apiName) {
			return
		}
		if listValuedParams.allows(apiName) {
			val = dedupeCSV(val)
		}
		p.Set(apiName, val)
	}
	if lf.limit > 0 && allowed.allows("limit") {
		p.Set("limit", strconv.Itoa(lf.limit))
	}
	add("cursor", lf.cursor)
	add("direction", lf.direction)
	add("countries", lf.countries)
	add("exchanges", lf.exchanges)
	add("tickers", lf.tickers)
	add("isins", lf.isins)
	add("ciks", lf.ciks)
	add("openfigis", lf.openfigis)
	if companyEndpoint {
		// Both flags feed the same parameter here, so merge them; setting
		// them one after the other would silently drop --company-ids.
		add("ids", strings.Join(append(parseCSV(lf.companyIDs), parseCSV(lf.ids)...), ","))
	} else {
		add("companyIds", lf.companyIDs)
	}
	add("startDate", lf.startDate)
	add("endDate", lf.endDate)
	add("updatedAfter", lf.updatedAfter)
	add("updatedBefore", lf.updatedBefore)
	add("typeIds", lf.typeIDs)
	add("eventIds", lf.eventIDs)
	add("documentGroupIds", lf.documentGroupIDs)
	add("expand", lf.expand)
	add("states", lf.states)
	add("transcriptVersion", lf.transcriptVersion)
	add("sortBy", lf.sortBy)
	add("levels", lf.levels)
	return p
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	return fs
}

func parseInterspersed(fs *flag.FlagSet, args []string) error {
	// The standard flag package stops parsing flags at the first positional
	// argument. CLI users usually expect `cmd <id> --flag value` to work, so we
	// move recognized flags before positionals before delegating to flag.Parse.
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positionals = append(positionals, a)
			continue
		}

		nameVal := strings.TrimLeft(a, "-")
		name := nameVal
		if eq := strings.Index(nameVal, "="); eq >= 0 {
			name = nameVal[:eq]
		}
		flags = append(flags, a)
		if strings.Contains(nameVal, "=") {
			continue
		}
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", a)
			}
			i++
			flags = append(flags, args[i])
		}
	}
	return fs.Parse(append(flags, positionals...))
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface{ IsBoolFlag() bool }
	bf, ok := f.Value.(boolFlag)
	return ok && bf.IsBoolFlag()
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func flagWasPassed(args []string, name string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--"+name || strings.HasPrefix(a, "--"+name+"=") {
			return true
		}
	}
	return false
}
