package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"quartr-cli/internal/output"
	"quartr-cli/internal/quartr"
)

func (a *app) handleAuth(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		a.printAuthHelp()
		return nil
	}

	switch args[0] {
	case "login", "set-key":
		fs := newFlagSet("auth login", a.errOut)
		apiKeyStdin := fs.Bool("api-key-stdin", false, "read API key from stdin (one line, trimmed)")
		baseURL := fs.String("base-url", a.cfg.BaseURL(), "API base URL")
		format := fs.String("format", a.cfg.Format(), "default output format")
		timeout := fs.String("timeout", a.cfg.Timeout(), "default HTTP timeout, e.g. 30s")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}

		// Source priority: --api-key-stdin > resolved config (global flag/env/file) > interactive prompt.
		// The global --api-key flag is honored via a.cfg.APIKey() but discouraged for `auth login`
		// because the literal value leaks via argv (ps, history, scrollback, CI logs).
		var key string
		switch {
		case *apiKeyStdin:
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("read api key from stdin: %w", err)
			}
			key = strings.TrimSpace(line)
		default:
			key = strings.TrimSpace(a.cfg.APIKey())
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
		if err := quartr.SaveConfig(a.cfg.ConfigPath(), cfg); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Saved API key to %s (%s)\n", a.cfg.ConfigPath(), quartr.MaskKey(key))
		return nil
	case "show":
		fmt.Fprintf(a.out, "config:   %s\n", a.cfg.ConfigPath())
		fmt.Fprintf(a.out, "api key:  %s\n", quartr.MaskKey(a.cfg.APIKey()))
		fmt.Fprintf(a.out, "base url: %s\n", a.cfg.BaseURL())
		fmt.Fprintf(a.out, "format:   %s\n", a.cfg.Format())
		fmt.Fprintf(a.out, "timeout:  %s\n", a.cfg.Timeout())
		return nil
	case "logout", "clear":
		if err := os.Remove(a.cfg.ConfigPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintf(a.out, "Removed %s\n", a.cfg.ConfigPath())
		return nil
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
}

func (a *app) handleResource(r resource, args []string) error {
	if r.name == "" {
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
	case "resolve":
		return a.resolveResource(r, rest)
	case "summary", "summarize":
		return a.summaryResource(r, rest)
	case "pages":
		return a.childListResource(r, r.pagesPath, params("limit", "cursor", "direction"), rest, "pages")
	case "chapters":
		return a.childListResource(r, r.chaptersPath, params("limit", "cursor", "direction", "levels"), rest, "chapters")
	case "download", "dl":
		return a.downloadResource(r, rest)
	case "stream":
		return a.streamResource(r, rest)
	default:
		return fmt.Errorf("unknown %s operation %q; run `quartr %s --help`", r.name, op, r.name)
	}
}

func (a *app) listResource(r resource, args []string) error {
	if r.listPath == "" {
		return fmt.Errorf("%s does not have a list endpoint", r.name)
	}
	fs := newFlagSet(r.name+" list", a.errOut)
	lf := listFlags{}
	addListFlags(fs, &lf)
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if err := validateSortBy(r, lf.sortBy); err != nil {
		return err
	}
	var joinCompany bool
	lf.expand, joinCompany = splitExpand(lf.expand)
	if joinCompany {
		if err := checkCompanyExpand(r); err != nil {
			return err
		}
	}
	if err := a.applyQualifiedTickers(context.Background(), r, &lf); err != nil {
		return err
	}
	// A lookup table is only useful whole: the type ids people need most
	// (25 = shareholder letter, 46 = DEFM14A) live past the default page.
	if r.fullCatalog && !flagWasPassed(args, "limit") && !flagWasPassed(args, "cursor") {
		lf.all = true
	}
	if lf.all && !flagWasPassed(args, "limit") {
		lf.limit = 500
	}

	return a.fetchList(listRequest{
		path:        r.listPath,
		params:      lf.toParams(r.listParams, r.name == "companies"),
		all:         lf.all,
		fields:      parseCSV(lf.fields),
		joinCompany: joinCompany,
	})
}

func (a *app) getResource(r resource, args []string) error {
	if r.getPath == "" {
		return fmt.Errorf("%s does not have a get endpoint", r.name)
	}
	fs := newFlagSet(r.name+" get", a.errOut)
	fields := fs.String("fields", "", "comma-separated output fields")
	expand := fs.String("expand", "", "fields to expand: event (API) or company (joined client-side)")
	transcriptVersion := fs.String("transcript-version", "", "live transcript version")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s get <id>", r.name)
	}
	apiExpand, joinCompany := splitExpand(*expand)
	if joinCompany {
		if err := checkCompanyExpand(r); err != nil {
			return err
		}
	}

	params := url.Values{}
	if r.getParams.allows("expand") && apiExpand != "" {
		params.Set("expand", apiExpand)
	}
	if r.getParams.allows("transcriptVersion") && *transcriptVersion != "" {
		params.Set("transcriptVersion", *transcriptVersion)
	}

	ctx := context.Background()
	path := strings.ReplaceAll(r.getPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(ctx, path, params)
	if err != nil {
		return err
	}
	if joinCompany {
		a.joinCompanies(ctx, joinableRows(obj))
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: parseCSV(*fields)})
}

// resolveResource implements `quartr companies resolve <ticker|cik>`: the one
// step that turns an ambiguous ticker into a companyId you can trust. Quartr
// matches tickers across every exchange, so this prints every candidate with
// the exchange pairs that matched rather than guessing which one was meant.
func (a *app) resolveResource(r resource, args []string) error {
	if r.name != "companies" {
		return usagef("`resolve` is only available on `quartr companies`")
	}
	fs := newFlagSet("companies resolve", a.errOut)
	fields := fs.String("fields", "", "comma-separated output fields")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usagef("usage: quartr companies resolve <ticker|cik>   (e.g. BLD, NYSE:BLD, 0001739445)")
	}

	query := strings.TrimSpace(fs.Arg(0))
	if query == "" || strings.ContainsAny(query, " \t") {
		return usagef("the Quartr API has no company name search; pass a ticker (BLD or NYSE:BLD) or a CIK")
	}

	ctx := context.Background()
	var companies []map[string]any
	var err error
	if looksLikeCIK(query) {
		companies, err = a.lookupCompanies(ctx, "ciks", query, nil)
	} else {
		specs := parseTickerSpecs(query)
		companies, err = a.lookupCompanies(ctx, "tickers", bareTickerCSV(specs), specs)
	}
	if err != nil {
		return err
	}
	if len(companies) == 0 {
		return fmt.Errorf("no company matches %q", query)
	}

	chosen := parseCSV(*fields)
	if len(chosen) == 0 {
		chosen = []string{"id", "name", "country", "matchedTickers"}
	}
	result := map[string]any{"data": companies, "count": len(companies)}
	return output.Write(a.out, result, output.Options{Format: a.cfg.Format(), Fields: chosen})
}

func (a *app) summaryResource(r resource, args []string) error {
	if r.summaryPath == "" {
		return fmt.Errorf("%s does not have a summary endpoint", r.name)
	}
	fs := newFlagSet(r.name+" summary", a.errOut)
	length := fs.String("length", "", "summary length: line, short, or long")
	plain := fs.Bool("plain", false, "plain text without embedded document sources")
	fields := fs.String("fields", "", "comma-separated output fields")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s summary <id>", r.name)
	}

	params := url.Values{}
	if r.summaryParams.allows("length") && *length != "" {
		params.Set("length", *length)
	}
	if r.summaryParams.allows("plain") && *plain {
		params.Set("plain", "true")
	}

	path := strings.ReplaceAll(r.summaryPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: parseCSV(*fields)})
}

func (a *app) childListResource(r resource, pathTpl string, allowed paramSet, args []string, child string) error {
	if pathTpl == "" {
		return fmt.Errorf("%s does not have a %s endpoint", r.name, child)
	}
	fs := newFlagSet(r.name+" "+child, a.errOut)
	lf := listFlags{}
	addListFlags(fs, &lf)
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s %s <id>", r.name, child)
	}
	if strings.TrimSpace(lf.sortBy) != "" {
		return usagef("--sort-by is not supported by `quartr %s %s`; rows are returned in document order",
			r.name, child)
	}
	if lf.all && !flagWasPassed(args, "limit") {
		lf.limit = 500
	}

	return a.fetchList(listRequest{
		path:   strings.ReplaceAll(pathTpl, "{id}", url.PathEscape(fs.Arg(0))),
		params: lf.toParams(allowed, false),
		all:    lf.all,
		fields: parseCSV(lf.fields),
	})
}

func (a *app) downloadResource(r resource, args []string) error {
	if r.downloadField == "" {
		return fmt.Errorf("%s does not have a configured download URL field", r.name)
	}
	fs := newFlagSet(r.name+" download", a.errOut)
	outPath := fs.String("output", "", "output file path, or - to stream to stdout; defaults to a name based on id and URL")
	urlField := fs.String("url-field", r.downloadField, "metadata URL field to download")
	withAPIKey := fs.Bool("with-api-key", false, "include x-api-key when fetching the file URL")
	expand := fs.String("expand", "", "fields to expand on the metadata request")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s download <id> [--output file]", r.name)
	}

	id := fs.Arg(0)
	path := strings.ReplaceAll(r.getPath, "{id}", url.PathEscape(id))
	params := url.Values{}
	if r.getParams.allows("expand") && *expand != "" {
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

	apiKey := ""
	if *withAPIKey {
		apiKey = a.cfg.APIKey()
	}

	// `--output -` streams the document itself to stdout so it can be piped
	// or redirected. Everything else this command prints goes to stderr, so
	// `quartr transcripts download <id> --output - > f.json` writes the
	// document and nothing else.
	if *outPath == "-" {
		_, err := a.client.Download(context.Background(), downloadURL, apiKey, a.out)
		return err
	}

	dest := *outPath
	if dest == "" {
		dest = defaultFileName(r.name, id, downloadURL)
	}
	if dir := filepath.Dir(dest); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := a.client.Download(context.Background(), downloadURL, apiKey, f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	// stderr, not stdout: a redirect is supposed to capture the document.
	fmt.Fprintf(a.errOut, "Saved %s\n", dest)
	return nil
}

func (a *app) streamResource(r resource, args []string) error {
	if r.streamField == "" {
		return fmt.Errorf("%s does not have a configured stream field", r.name)
	}
	fs := newFlagSet(r.name+" stream", a.errOut)
	transcriptVersion := fs.String("transcript-version", "1.7", "live transcript stream version")
	withAPIKey := fs.Bool("with-api-key", false, "include x-api-key when opening the stream URL")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s stream <id>", r.name)
	}

	params := url.Values{}
	if *transcriptVersion != "" {
		params.Set("transcriptVersion", *transcriptVersion)
	}
	path := strings.ReplaceAll(r.getPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	streamURL, err := extractStringField(obj, r.streamField)
	if err != nil {
		return err
	}

	apiKey := ""
	if *withAPIKey {
		apiKey = a.cfg.APIKey()
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
		return a.fetchList(listRequest{path: fs.Arg(0), params: params, all: true, fields: parseCSV(*fields)})
	}

	obj, _, err := a.client.GetJSON(context.Background(), fs.Arg(0), params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: parseCSV(*fields)})
}
