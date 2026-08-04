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
	if lf.all && !flagWasPassed(args, "limit") {
		lf.limit = 500
	}

	params := lf.toParams(r.listParams, r.name == "companies")
	fields := parseCSV(lf.fields)
	return a.fetchList(r.listPath, params, lf.all, fields)
}

func (a *app) getResource(r resource, args []string) error {
	if r.getPath == "" {
		return fmt.Errorf("%s does not have a get endpoint", r.name)
	}
	fs := newFlagSet(r.name+" get", a.errOut)
	fields := fs.String("fields", "", "comma-separated output fields")
	expand := fs.String("expand", "", "fields to expand, e.g. event")
	transcriptVersion := fs.String("transcript-version", "", "live transcript version")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: quartr %s get <id>", r.name)
	}

	params := url.Values{}
	if r.getParams.allows("expand") && *expand != "" {
		params.Set("expand", *expand)
	}
	if r.getParams.allows("transcriptVersion") && *transcriptVersion != "" {
		params.Set("transcriptVersion", *transcriptVersion)
	}

	path := strings.ReplaceAll(r.getPath, "{id}", url.PathEscape(fs.Arg(0)))
	obj, _, err := a.client.GetJSON(context.Background(), path, params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: parseCSV(*fields)})
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

	params := lf.toParams(allowed, false)
	path := strings.ReplaceAll(pathTpl, "{id}", url.PathEscape(fs.Arg(0)))
	return a.fetchList(path, params, lf.all, parseCSV(lf.fields))
}

func (a *app) downloadResource(r resource, args []string) error {
	if r.downloadField == "" {
		return fmt.Errorf("%s does not have a configured download URL field", r.name)
	}
	fs := newFlagSet(r.name+" download", a.errOut)
	outPath := fs.String("output", "", "output file path; defaults to a name based on id and URL")
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

	dest := *outPath
	if dest == "" {
		dest = defaultFileName(r.name, id, downloadURL)
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
		apiKey = a.cfg.APIKey()
	}
	if _, err := a.client.Download(context.Background(), downloadURL, apiKey, f); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Saved %s\n", dest)
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
		return a.fetchList(fs.Arg(0), params, true, parseCSV(*fields))
	}

	obj, _, err := a.client.GetJSON(context.Background(), fs.Arg(0), params)
	if err != nil {
		return err
	}
	return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: parseCSV(*fields)})
}
