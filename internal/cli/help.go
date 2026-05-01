package cli

import (
	"fmt"
	"strings"

	"quartr-cli/internal/quartr"
)

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
	if r.listPath != "" {
		ops = append(ops, "list")
	}
	if r.getPath != "" {
		ops = append(ops, "get <id>")
	}
	if r.summaryPath != "" {
		ops = append(ops, "summary <id>")
	}
	if r.pagesPath != "" {
		ops = append(ops, "pages <id>")
	}
	if r.chaptersPath != "" {
		ops = append(ops, "chapters <id>")
	}
	if r.downloadField != "" {
		ops = append(ops, "download <id>")
	}
	if r.streamField != "" {
		ops = append(ops, "stream <id>")
	}

	fmt.Fprintf(a.out, "Usage:\n  quartr %s <%s> [flags]\n\nOperations:\n", r.name, strings.Join(ops, " | "))
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
	switch r.name {
	case "companies":
		fmt.Fprint(a.out, "  quartr companies list --tickers AAPL\n  quartr companies get 4742 --format json\n")
	case "events":
		fmt.Fprint(a.out, "  quartr events list --tickers AAPL --sort-by date --direction desc\n  quartr events summary 128301 --length long --plain\n")
	case "transcripts":
		fmt.Fprint(a.out, "  quartr transcripts list --tickers AAPL --expand event\n  quartr transcripts download 432907 --output transcript.json\n")
	case "live-transcripts":
		fmt.Fprint(a.out, "  quartr live transcripts list --states live,willBeLive\n  quartr live transcripts stream 127537 --transcript-version 1.7\n")
	default:
		fmt.Fprintf(a.out, "  quartr %s list --limit 5\n", r.name)
	}
}
