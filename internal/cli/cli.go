package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"quartr-cli/internal/output"
	"quartr-cli/internal/quartr"
)

const Version = "0.1.0"

type globalOverrides struct {
	APIKey   *string
	BaseURL  *string
	Config   *string
	Format   *string
	Timeout  *string
	NoConfig bool
	Debug    bool
	Help     bool
	Version  bool
	RawArgs  []string
	Stripped []string
}

type app struct {
	out    io.Writer
	errOut io.Writer
	cfg    effectiveConfig
	client *quartr.Client
}

type effectiveConfig struct {
	APIKey     string
	BaseURL    string
	Format     string
	Timeout    string
	ConfigPath string
	NoConfig   bool
	Debug      bool
}

type resource struct {
	Name          string
	Plural        string
	ListPath      string
	GetPath       string
	SummaryPath   string
	PagesPath     string
	ChaptersPath  string
	DownloadField string
	StreamField   string
	ListParams    map[string]bool
	GetParams     map[string]bool
	SummaryParams map[string]bool
	NestedHelp    string
}

var resources = map[string]resource{}

func init() {
	baseList := names("countries", "exchanges", "tickers", "isins", "ciks", "companyIds", "startDate", "endDate", "updatedAfter", "updatedBefore", "limit", "cursor", "direction")
	docList := mergeParams(baseList, names("typeIds", "eventIds", "documentGroupIds", "expand"))
	audioList := mergeParams(baseList, names("eventIds", "expand"))
	liveList := mergeParams(names("countries", "exchanges", "tickers", "isins", "ciks", "companyIds", "eventIds", "states", "startDate", "endDate", "updatedAfter", "updatedBefore", "limit", "cursor", "direction"), names("transcriptVersion"))
	liveAudioList := names("countries", "exchanges", "tickers", "isins", "ciks", "companyIds", "eventIds", "states", "startDate", "endDate", "updatedAfter", "updatedBefore", "limit", "cursor", "direction")
	simpleList := names("limit", "cursor", "direction")
	companyList := names("countries", "exchanges", "tickers", "isins", "ciks", "ids", "updatedAfter", "updatedBefore", "limit", "cursor", "direction")
	summaryParams := names("length", "plain")
	getExpand := names("expand")
	getLiveTranscript := names("transcriptVersion")

	addResource(resource{Name: "companies", Plural: "companies", ListPath: "/companies", GetPath: "/companies/{id}", ListParams: companyList})
	addResource(resource{Name: "events", Plural: "events", ListPath: "/events", GetPath: "/events/{id}", SummaryPath: "/events/{id}/summary", ListParams: mergeParams(baseList, names("typeIds", "sortBy")), SummaryParams: summaryParams})
	addResource(resource{Name: "documents", Plural: "documents", ListPath: "/documents", GetPath: "/documents/{id}", DownloadField: "fileUrl", ListParams: docList, GetParams: getExpand})
	addResource(resource{Name: "reports", Plural: "reports", ListPath: "/documents/reports", GetPath: "/documents/reports/{id}", PagesPath: "/documents/reports/{id}/pages", SummaryPath: "/documents/reports/{id}/summary", DownloadField: "fileUrl", ListParams: docList, GetParams: getExpand, SummaryParams: summaryParams})
	addResource(resource{Name: "slides", Plural: "slide decks", ListPath: "/documents/slides", GetPath: "/documents/slides/{id}", PagesPath: "/documents/slides/{id}/pages", SummaryPath: "/documents/slides/{id}/summary", DownloadField: "fileUrl", ListParams: docList, GetParams: getExpand, SummaryParams: summaryParams})
	addResource(resource{Name: "transcripts", Plural: "transcripts", ListPath: "/documents/transcripts", GetPath: "/documents/transcripts/{id}", ChaptersPath: "/documents/transcripts/{id}/chapters", SummaryPath: "/documents/transcripts/{id}/summary", DownloadField: "fileUrl", ListParams: docList, GetParams: getExpand, SummaryParams: summaryParams})
	addResource(resource{Name: "audio", Plural: "audio", ListPath: "/audio", GetPath: "/audio/{id}", ChaptersPath: "/audio/{id}/chapters", DownloadField: "fileUrl", ListParams: audioList, GetParams: getExpand})
	addResource(resource{Name: "live", Plural: "live events", ListPath: "/live", GetPath: "/live/{id}", ListParams: liveList, GetParams: getLiveTranscript})
	addResource(resource{Name: "live-audio", Plural: "live audio", ListPath: "/live/audio", GetPath: "/live/audio/{id}", DownloadField: "audio", ListParams: liveAudioList})
	addResource(resource{Name: "live-transcripts", Plural: "live transcripts", ListPath: "/live/transcripts", GetPath: "/live/transcripts/{id}", DownloadField: "transcript", StreamField: "transcript", ListParams: liveList, GetParams: getLiveTranscript})
	addResource(resource{Name: "event-types", Plural: "event types", ListPath: "/event-types", GetPath: "/event-types/{id}", ListParams: simpleList})
	addResource(resource{Name: "document-types", Plural: "document types", ListPath: "/document-types", GetPath: "/document-types/{id}", ListParams: simpleList})
}

func addResource(r resource) { resources[r.Name] = r }
func names(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
func mergeParams(ms ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		maps.Copy(out, m)
	}
	return out
}

func Run(args []string, out, errOut io.Writer) int {
	globals, commandArgs, err := extractGlobalFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if globals.Version {
		fmt.Fprintf(out, "quartr %s\n", Version)
		return 0
	}
	cfg, err := buildConfig(globals)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	a := &app{out: out, errOut: errOut, cfg: cfg}
	a.client = quartr.NewClient(cfg.BaseURL, cfg.APIKey, quartr.EffectiveTimeout(cfg.Timeout), cfg.Debug)
	if len(commandArgs) == 0 {
		a.printRootHelp()
		return 0
	}
	if globals.Help {
		commandArgs = append(commandArgs, "--help")
	}
	if err := a.dispatch(commandArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

func buildConfig(g globalOverrides) (effectiveConfig, error) {
	cfg := effectiveConfig{
		BaseURL:    quartr.DefaultBaseURL,
		Format:     "table",
		Timeout:    "30s",
		ConfigPath: quartr.DefaultConfigPath(),
		NoConfig:   g.NoConfig,
		Debug:      g.Debug,
	}
	if g.Config != nil && strings.TrimSpace(*g.Config) != "" {
		cfg.ConfigPath = *g.Config
	}
	if !cfg.NoConfig {
		fileCfg, err := quartr.LoadConfig(cfg.ConfigPath)
		if err != nil {
			return cfg, err
		}
		if fileCfg.APIKey != "" {
			cfg.APIKey = fileCfg.APIKey
		}
		if fileCfg.BaseURL != "" {
			cfg.BaseURL = fileCfg.BaseURL
		}
		if fileCfg.Format != "" {
			cfg.Format = fileCfg.Format
		}
		if fileCfg.Timeout != "" {
			cfg.Timeout = fileCfg.Timeout
		}
	}
	if v := os.Getenv("QUARTR_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("QUARTR_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("QUARTR_FORMAT"); v != "" {
		cfg.Format = v
	}
	if v := os.Getenv("QUARTR_TIMEOUT"); v != "" {
		cfg.Timeout = v
	}
	if g.APIKey != nil {
		cfg.APIKey = *g.APIKey
	}
	if g.BaseURL != nil {
		cfg.BaseURL = *g.BaseURL
	}
	if g.Format != nil {
		cfg.Format = *g.Format
	}
	if g.Timeout != nil {
		cfg.Timeout = *g.Timeout
	}
	return cfg, nil
}

func (a *app) dispatch(args []string) error {
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "help":
		if len(rest) == 0 {
			a.printRootHelp()
			return nil
		}
		return a.dispatch(append(rest, "--help"))
	case "auth":
		return a.handleAuth(rest)
	case "request", "raw":
		return a.handleRequest(rest)
	case "companies", "events", "documents", "reports", "slides", "transcripts", "audio", "event-types", "document-types", "live-audio", "live-transcripts":
		return a.handleResource(resources[cmd], rest)
	case "live":
		if len(rest) > 0 {
			sub := rest[0]
			if sub == "audio" {
				return a.handleResource(resources["live-audio"], rest[1:])
			}
			if sub == "transcripts" || sub == "transcript" {
				return a.handleResource(resources["live-transcripts"], rest[1:])
			}
		}
		return a.handleResource(resources["live"], rest)
	default:
		return fmt.Errorf("unknown command %q; run `quartr help`", cmd)
	}
}

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
		name := nameVal
		val := ""
		hasEq := false
		if eq := strings.Index(nameVal, "="); eq >= 0 {
			name = nameVal[:eq]
			val = nameVal[eq+1:]
			hasEq = true
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

func (a *app) handleAuth(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		a.printAuthHelp()
		return nil
	}
	switch args[0] {
	case "login", "set-key":
		fs := newFlagSet("auth login", a.errOut)
		apiKey := fs.String("api-key", "", "Quartr API key")
		baseURL := fs.String("base-url", a.cfg.BaseURL, "API base URL")
		format := fs.String("format", a.cfg.Format, "default output format")
		timeout := fs.String("timeout", a.cfg.Timeout, "default HTTP timeout, e.g. 30s")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		key := strings.TrimSpace(*apiKey)
		if key == "" {
			key = strings.TrimSpace(a.cfg.APIKey)
		}
		if key == "" {
			fmt.Fprint(a.out, "API key: ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			key = strings.TrimSpace(line)
		}
		if key == "" {
			return errors.New("empty API key")
		}
		cfg := quartr.Config{APIKey: key, BaseURL: *baseURL, Format: *format, Timeout: *timeout}
		if err := quartr.SaveConfig(a.cfg.ConfigPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Saved API key to %s (%s)\n", a.cfg.ConfigPath, quartr.MaskKey(key))
		return nil
	case "show":
		fmt.Fprintf(a.out, "config:   %s\n", a.cfg.ConfigPath)
		fmt.Fprintf(a.out, "api key:  %s\n", quartr.MaskKey(a.cfg.APIKey))
		fmt.Fprintf(a.out, "base url: %s\n", a.cfg.BaseURL)
		fmt.Fprintf(a.out, "format:   %s\n", a.cfg.Format)
		fmt.Fprintf(a.out, "timeout:  %s\n", a.cfg.Timeout)
		return nil
	case "logout", "clear":
		if err := os.Remove(a.cfg.ConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintf(a.out, "Removed %s\n", a.cfg.ConfigPath)
		return nil
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
}

func (a *app) handleResource(r resource, args []string) error {
	if r.Name == "" {
		return errors.New("internal error: missing resource")
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		a.printResourceHelp(r)
		return nil
	}
	op := args[0]
	rest := args[1:]
	switch op {
	case "list", "ls":
		return a.listResource(r, rest)
	case "get", "show":
		return a.getResource(r, rest)
	case "summary", "summarize":
		return a.summaryResource(r, rest)
	case "pages":
		return a.childListResource(r, r.PagesPath, names("limit", "cursor", "direction"), rest, "pages")
	case "chapters":
		return a.childListResource(r, r.ChaptersPath, names("limit", "cursor", "direction", "levels"), rest, "chapters")
	case "download", "dl":
		return a.downloadResource(r, rest)
	case "stream":
		return a.streamResource(r, rest)
	default:
		return fmt.Errorf("unknown %s operation %q; run `quartr %s --help`", r.Name, op, r.Name)
	}
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
	fs.StringVar(&lf.tickers, "tickers", "", "comma-separated tickers, e.g. AAPL,MSFT")
	fs.StringVar(&lf.isins, "isins", "", "comma-separated ISINs")
	fs.StringVar(&lf.ciks, "ciks", "", "comma-separated SEC CIKs")
	fs.StringVar(&lf.companyIDs, "company-ids", "", "comma-separated Quartr company IDs")
	fs.StringVar(&lf.ids, "ids", "", "comma-separated IDs for resources that support ids")
	fs.StringVar(&lf.startDate, "start-date", "", "ISO 8601 start date")
	fs.StringVar(&lf.endDate, "end-date", "", "ISO 8601 end date")
	fs.StringVar(&lf.updatedAfter, "updated-after", "", "ISO 8601 updatedAfter")
	fs.StringVar(&lf.updatedBefore, "updated-before", "", "ISO 8601 updatedBefore")
	fs.StringVar(&lf.typeIDs, "type-ids", "", "comma-separated type IDs")
	fs.StringVar(&lf.eventIDs, "event-ids", "", "comma-separated event IDs")
	fs.StringVar(&lf.documentGroupIDs, "document-group-ids", "", "comma-separated document group IDs")
	fs.StringVar(&lf.expand, "expand", "", "comma-separated fields to expand, e.g. event")
	fs.StringVar(&lf.states, "states", "", "comma-separated live states")
	fs.StringVar(&lf.transcriptVersion, "transcript-version", "", "live transcript stream version, e.g. 1.7")
	fs.StringVar(&lf.sortBy, "sort-by", "", "sort field for endpoints that support it")
	fs.StringVar(&lf.levels, "levels", "", "comma-separated chapter levels")
}

func (a *app) listResource(r resource, args []string) error {
	if r.ListPath == "" {
		return fmt.Errorf("%s does not have a list endpoint", r.Name)
	}
	fs := newFlagSet(r.Name+" list", a.errOut)
	lf := listFlags{}
	addListFlags(fs, &lf)
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if lf.all && !flagWasPassed(args, "limit") {
		lf.limit = 500
	}
	params := lf.toParams(r.ListParams, r.Name == "companies")
	fields := parseCSV(lf.fields)
	return a.fetchList(r.ListPath, params, lf.all, fields)
}

func (lf listFlags) toParams(allowed map[string]bool, companyEndpoint bool) url.Values {
	p := url.Values{}
	add := func(apiName, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		if allowed[apiName] {
			p.Set(apiName, val)
		}
	}
	if lf.limit > 0 && allowed["limit"] {
		p.Set("limit", fmt.Sprintf("%d", lf.limit))
	}
	add("cursor", lf.cursor)
	add("direction", lf.direction)
	add("countries", lf.countries)
	add("exchanges", lf.exchanges)
	add("tickers", lf.tickers)
	add("isins", lf.isins)
	add("ciks", lf.ciks)
	if companyEndpoint {
		if lf.companyIDs != "" {
			add("ids", lf.companyIDs)
		}
		add("ids", lf.ids)
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

func (a *app) fetchList(path string, params url.Values, all bool, fields []string) error {
	ctx := context.Background()
	if !all {
		obj, _, err := a.client.GetJSON(ctx, path, params)
		if err != nil {
			return err
		}
		return output.Write(a.out, obj, output.Options{Format: a.cfg.Format, Fields: fields})
	}
	allRows := make([]map[string]any, 0)
	var finalPagination any
	seen := map[string]bool{}
	for {
		cursor := params.Get("cursor")
		if cursor != "" {
			if seen[cursor] {
				return fmt.Errorf("pagination loop detected at cursor %q", cursor)
			}
			seen[cursor] = true
		}
		obj, _, err := a.client.GetJSON(ctx, path, params)
		if err != nil {
			return err
		}
		if p, ok := obj["pagination"]; ok {
			finalPagination = p
		}
		rows := dataRows(obj)
		allRows = append(allRows, rows...)
		next := nextCursor(obj)
		if next == "" {
			break
		}
		params.Set("cursor", next)
	}
	wrapped := map[string]any{"data": allRows, "pagination": finalPagination, "count": len(allRows)}
	return output.Write(a.out, wrapped, output.Options{Format: a.cfg.Format, Fields: fields})
}

func dataRows(obj map[string]any) []map[string]any {
	data, ok := obj["data"]
	if !ok {
		return nil
	}
	arr, ok := data.([]any)
	if !ok {
		return nil
	}
	rows := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

func nextCursor(obj map[string]any) string {
	p, ok := obj["pagination"].(map[string]any)
	if !ok {
		return ""
	}
	v := p["nextCursor"]
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	if s == "<nil>" || s == "" {
		return ""
	}
	return s
}

func (a *app) getResource(r resource, args []string) error {
	if r.GetPath == "" {
		return fmt.Errorf("%s does not have a get endpoint", r.Name)
	}
	fs := newFlagSet(r.Name+" get", a.errOut)
	fields := fs.String("fields", "", "comma-separated output fields")
	expand := fs.String("expand", "", "fields to expand, e.g. event")
	transcriptVersion := fs.String("transcript-version", "", "live transcript version")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s get <id>", r.Name)
	}
	params := url.Values{}
	if r.GetParams["expand"] && *expand != "" {
		params.Set("expand", *expand)
	}
	if r.GetParams["transcriptVersion"] && *transcriptVersion != "" {
		params.Set("transcriptVersion", *transcriptVersion)
	}
	path := strings.ReplaceAll(r.GetPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format, Fields: parseCSV(*fields)})
}

func (a *app) summaryResource(r resource, args []string) error {
	if r.SummaryPath == "" {
		return fmt.Errorf("%s does not have a summary endpoint", r.Name)
	}
	fs := newFlagSet(r.Name+" summary", a.errOut)
	length := fs.String("length", "", "summary length: line, short, or long")
	plain := fs.Bool("plain", false, "plain text without embedded document sources")
	fields := fs.String("fields", "", "comma-separated output fields")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s summary <id>", r.Name)
	}
	params := url.Values{}
	if r.SummaryParams["length"] && *length != "" {
		params.Set("length", *length)
	}
	if r.SummaryParams["plain"] && *plain {
		params.Set("plain", "true")
	}
	path := strings.ReplaceAll(r.SummaryPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format, Fields: parseCSV(*fields)})
}

func (a *app) childListResource(r resource, pathTpl string, allowed map[string]bool, args []string, child string) error {
	if pathTpl == "" {
		return fmt.Errorf("%s does not have a %s endpoint", r.Name, child)
	}
	fs := newFlagSet(r.Name+" "+child, a.errOut)
	lf := listFlags{}
	addListFlags(fs, &lf)
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s %s <id>", r.Name, child)
	}
	if lf.all && !flagWasPassed(args, "limit") {
		lf.limit = 500
	}
	params := lf.toParams(allowed, false)
	path := strings.ReplaceAll(pathTpl, "{id}", url.PathEscape(fs.Arg(0)))
	return a.fetchList(path, params, lf.all, parseCSV(lf.fields))
}

func (a *app) downloadResource(r resource, args []string) error {
	if r.DownloadField == "" {
		return fmt.Errorf("%s does not have a configured download URL field", r.Name)
	}
	fs := newFlagSet(r.Name+" download", a.errOut)
	outPath := fs.String("output", "", "output file path; defaults to a name based on id and URL")
	urlField := fs.String("url-field", r.DownloadField, "metadata URL field to download")
	withAPIKey := fs.Bool("with-api-key", false, "include x-api-key when fetching the file URL")
	expand := fs.String("expand", "", "fields to expand on the metadata request")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s download <id> [--output file]", r.Name)
	}
	id := fs.Arg(0)
	path := strings.ReplaceAll(r.GetPath, "{id}", url.PathEscape(id))
	params := url.Values{}
	if r.GetParams["expand"] && *expand != "" {
		params.Set("expand", *expand)
	}
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	downloadURL, err := extractStringField(obj, *urlField)
	if err != nil {
		return err
	}
	dest := *outPath
	if dest == "" {
		dest = defaultFileName(r.Name, id, downloadURL)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil && filepath.Dir(dest) != "." {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	apiKey := ""
	if *withAPIKey {
		apiKey = a.cfg.APIKey
	}
	if _, err := a.client.Download(context.Background(), downloadURL, apiKey, f); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Saved %s\n", dest)
	return nil
}

func (a *app) streamResource(r resource, args []string) error {
	if r.StreamField == "" {
		return fmt.Errorf("%s does not have a configured stream field", r.Name)
	}
	fs := newFlagSet(r.Name+" stream", a.errOut)
	transcriptVersion := fs.String("transcript-version", "1.7", "live transcript stream version")
	withAPIKey := fs.Bool("with-api-key", false, "include x-api-key when opening the stream URL")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s stream <id>", r.Name)
	}
	params := url.Values{}
	if *transcriptVersion != "" {
		params.Set("transcriptVersion", *transcriptVersion)
	}
	path := strings.ReplaceAll(r.GetPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	streamURL, err := extractStringField(obj, r.StreamField)
	if err != nil {
		return err
	}
	apiKey := ""
	if *withAPIKey {
		apiKey = a.cfg.APIKey
	}
	_, err = a.client.Download(context.Background(), streamURL, apiKey, a.out)
	return err
}

func (a *app) handleRequest(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		a.printRequestHelp()
		return nil
	}
	method := strings.ToLower(args[0])
	if method != "get" {
		return errors.New("only GET is supported")
	}
	fs := newFlagSet("request get", a.errOut)
	queryPairs := multiFlag{}
	fs.Var(&queryPairs, "query", "query parameter as key=value; repeatable")
	paginate := fs.Bool("paginate", false, "follow pagination.nextCursor")
	fields := fs.String("fields", "", "comma-separated output fields")
	if err := parseInterspersed(fs, args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: quartr request get <path> [--query key=value]")
	}
	params := url.Values{}
	for _, pair := range queryPairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("--query expects key=value, got %q", pair)
		}
		params.Add(k, v)
	}
	if *paginate {
		return a.fetchList(fs.Arg(0), params, true, parseCSV(*fields))
	}
	obj, _, err := a.client.GetJSON(context.Background(), fs.Arg(0), params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format, Fields: parseCSV(*fields)})
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

func extractStringField(obj map[string]any, field string) (string, error) {
	if strings.TrimSpace(field) == "" {
		return "", errors.New("empty url field")
	}
	s, err := extractStringFieldOnce(obj, field)
	if err == nil {
		return s, nil
	}
	// Common Quartr shape is {"data": {"fileUrl": "..."}}. Retry under data
	// when caller passed a top-level field.
	if !strings.HasPrefix(field, "data.") {
		if s, retryErr := extractStringFieldOnce(obj, "data."+field); retryErr == nil {
			return s, nil
		}
	}
	return "", err
}

func extractStringFieldOnce(obj map[string]any, field string) (string, error) {
	cur := any(obj)
	for part := range strings.SplitSeq(field, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field %q not found", field)
		}
		cur = m[part]
		if cur == nil {
			return "", fmt.Errorf("field %q not found", field)
		}
	}
	if s, ok := cur.(string); ok && strings.TrimSpace(s) != "" {
		return s, nil
	}
	return "", fmt.Errorf("field %q is not a non-empty string", field)
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func defaultFileName(resourceName, id, rawURL string) string {
	ext := ""
	if u, err := url.Parse(rawURL); err == nil {
		ext = filepath.Ext(u.Path)
	}
	if ext == "" {
		ext = ".bin"
	}
	base := unsafeFilename.ReplaceAllString(resourceName+"-"+id, "-")
	return base + ext
}

func (a *app) printRootHelp() {
	cmds := []string{"auth", "companies", "events", "documents", "reports", "slides", "transcripts", "audio", "live", "live audio", "live transcripts", "event-types", "document-types", "request"}
	fmt.Fprintf(a.out, `quartr %s

A Quartr Public API CLI.

Usage:
  quartr [global flags] <command> [operation] [flags]

Global flags:
  --api-key KEY        Quartr API key; defaults to QUARTR_API_KEY or config
  --base-url URL       API base URL; defaults to %s
  --config PATH        config path; defaults to %s
  --format FORMAT      table, json, csv, or raw
  --timeout DURATION   HTTP timeout, e.g. 30s
  --no-config          ignore config file
  --version            print version
  -h, --help           show help

Commands:
  %s

Examples:
  quartr auth login --api-key "$QUARTR_API_KEY"
  quartr companies list --tickers AAPL --format json
  quartr events list --tickers AAPL --sort-by date --direction desc --limit 5
  quartr transcripts list --tickers MSFT --expand event --limit 10
  quartr transcripts download 432907 --output transcript.json
  quartr live transcripts stream 127537 --transcript-version 1.7
  quartr request get /events --query tickers=AAPL --query limit=3 --format json
`, Version, quartr.DefaultBaseURL, quartr.DefaultConfigPath(), strings.Join(cmds, ", "))
}

func (a *app) printAuthHelp() {
	fmt.Fprint(a.out, `Usage:
  quartr auth login --api-key KEY
  quartr auth show
  quartr auth logout

The login command stores a small JSON config file with mode 0600.
`)
}

func (a *app) printRequestHelp() {
	fmt.Fprint(a.out, `Usage:
  quartr request get <path> [--query key=value] [--paginate]

Examples:
  quartr request get /companies --query tickers=AAPL --format json
  quartr request get /documents/transcripts --query tickers=AAPL --query expand=event
`)
}

func (a *app) printResourceHelp(r resource) {
	ops := []string{}
	if r.ListPath != "" {
		ops = append(ops, "list")
	}
	if r.GetPath != "" {
		ops = append(ops, "get <id>")
	}
	if r.SummaryPath != "" {
		ops = append(ops, "summary <id>")
	}
	if r.PagesPath != "" {
		ops = append(ops, "pages <id>")
	}
	if r.ChaptersPath != "" {
		ops = append(ops, "chapters <id>")
	}
	if r.DownloadField != "" {
		ops = append(ops, "download <id>")
	}
	if r.StreamField != "" {
		ops = append(ops, "stream <id>")
	}
	fmt.Fprintf(a.out, "Usage:\n  quartr %s <%s> [flags]\n\nOperations:\n", r.Name, strings.Join(ops, " | "))
	for _, op := range ops {
		fmt.Fprintf(a.out, "  %s\n", op)
	}
	fmt.Fprint(a.out, `
Common list flags:
  --tickers AAPL,MSFT      filter by tickers where supported
  --company-ids 4742       filter by Quartr company IDs where supported
  --start-date ISO         content/event date lower bound where supported
  --end-date ISO           content/event date upper bound where supported
  --updated-after ISO      incremental sync lower bound
  --limit N                page size, max 500
  --all                    follow pagination.nextCursor
  --fields a,b,c           output fields for table/csv

Examples:
`)
	switch r.Name {
	case "companies":
		fmt.Fprint(a.out, "  quartr companies list --tickers AAPL\n  quartr companies get 4742 --format json\n")
	case "events":
		fmt.Fprint(a.out, "  quartr events list --tickers AAPL --sort-by date --direction desc\n  quartr events summary 128301 --length long --plain\n")
	case "transcripts":
		fmt.Fprint(a.out, "  quartr transcripts list --tickers AAPL --expand event\n  quartr transcripts download 432907 --output transcript.json\n")
	case "live-transcripts":
		fmt.Fprint(a.out, "  quartr live transcripts list --states live,willBeLive\n  quartr live transcripts stream 127537 --transcript-version 1.7\n")
	default:
		fmt.Fprintf(a.out, "  quartr %s list --limit 5\n", r.Name)
	}
}

