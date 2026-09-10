package cli

import (
	"fmt"
	"strings"

	"github.com/TJC-LP/quartr-cli/internal/quartr"
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
  quartr reports text 105446 --output apple-10k.md
  quartr live transcripts stream 127537 --transcript-version 1.7
  quartr request get /events --query tickers=AAPL --query limit=3 --format json
`, quartr.Version, quartr.DefaultBaseURL, quartr.DefaultConfigPath(), strings.Join(cmds, ", "))
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

// resourceOps lists the operations a resource supports, in the order help
// shows them: the ones the resource map enables plus the companies-only
// resolve.
func resourceOps(r resource) []string {
	ops := []string{}
	add := func(enabled bool, op string) {
		if enabled {
			ops = append(ops, op)
		}
	}
	add(r.listPath != "", "list")
	add(r.getPath != "", "get <id>")
	add(r.name == "companies", "resolve <ticker|cik|figi>")
	add(r.summaryPath != "", "summary <id>")
	add(r.pagesPath != "", "pages <id>")
	add(r.textPath != "", "text <id>")
	add(r.chaptersPath != "", "chapters <id>")
	add(r.segmentsPath != "", "segments <id>")
	add(r.downloadField != "", "download <id>")
	add(r.streamField != "", "stream <id>")
	return ops
}

func (a *app) printResourceHelp(r resource) {
	ops := resourceOps(r)
	fmt.Fprintf(a.out, "Usage:\n  quartr %s <%s> [flags]\n\nOperations:\n", r.name, strings.Join(ops, " | "))
	for _, op := range ops {
		fmt.Fprintf(a.out, "  %s\n", op)
	}
	fmt.Fprint(a.out, `
Common list flags:
  --tickers AAPL,MSFT      filter by tickers where supported; a bare ticker
                           matches on every exchange, so qualify it as
                           NYSE:BLD when the symbol is shared
  --company-ids 4742       filter by Quartr company IDs where supported
  --start-date ISO         content/event date lower bound where supported
  --end-date ISO           content/event date upper bound where supported
  --updated-after ISO      incremental sync lower bound
  --limit N                page size, max 500
  --all                    follow pagination.nextCursor
  --fields a,b,c           output fields for table/csv
`)
	if r.listPath != "" {
		fmt.Fprintf(a.out, "\nSorting:\n")
		if len(r.sortFields) > 0 {
			fmt.Fprintf(a.out, "  --sort-by %s [--direction asc|desc]\n", strings.Join(r.sortFields, "|"))
		} else {
			fmt.Fprintf(a.out, "  --sort-by is rejected here (the endpoint has no sortBy parameter).\n  %s\n",
				strings.ReplaceAll(sortRecipe(r), "\n", "\n  "))
		}
	}
	if r.fullCatalog {
		fmt.Fprintf(a.out, `
Catalog:
  `+"`quartr %s list`"+` returns the whole table by default: it is a lookup
  list, and the ids people need most sit past the first page. Pass --limit
  to page through it instead.
`, r.name)
	}
	if r.downloadField != "" {
		fmt.Fprintf(a.out, `
Downloads:
  download <id>               writes ./%s-<id>.<ext> and reports the path on stderr
  download <id> --output P    writes P
  download <id> --output -    streams the document to stdout, nothing else
`, r.name)
	}
	if r.textPath != "" {
		fmt.Fprint(a.out, `
Parsed text (Markdown, separate Quartr package; 403 without it):
  text <id>                   prints the parsed Markdown on stdout
  text <id> --output P        writes P instead
  text <id> --metadata        prints the envelope (textUrl, updatedAt) instead
`)
	}
	fmt.Fprint(a.out, `
Examples:
`)
	switch r.name {
	case "companies":
		fmt.Fprint(a.out, "  quartr companies list --tickers AAPL\n  quartr companies list --openfigis BBG000B9XRY4\n  quartr companies resolve BLD          # every company using that ticker\n  quartr companies get 4742 --format json\n  quartr companies segments 4742 --format json\n")
	case "reports":
		fmt.Fprint(a.out, "  quartr reports list --tickers AAPL --type-ids 11 --limit 5\n  quartr reports text 105446 | head -50\n  quartr reports text 105446 --output apple-10k.md\n  quartr reports download 105446 --output apple-10k.pdf\n")
	case "slides":
		fmt.Fprint(a.out, "  quartr slides list --tickers AAPL --limit 5\n  quartr slides text 152141 > deck.md\n  quartr slides pages 152141 --format csv\n")
	case "events":
		fmt.Fprint(a.out, "  quartr events list --tickers AAPL --sort-by date --direction desc\n  quartr events summary 128301 --length long --plain\n")
	case "transcripts":
		fmt.Fprint(a.out, "  quartr transcripts list --tickers AAPL --expand event\n  quartr transcripts download 432907 --output transcript.json\n  quartr transcripts download 432907 --output - | jq .\n")
	case "live-transcripts":
		fmt.Fprint(a.out, "  quartr live transcripts list --states live,willBeLive\n  quartr live transcripts stream 127537 --transcript-version 1.7\n")
	default:
		fmt.Fprintf(a.out, "  quartr %s list --limit 5\n", r.name)
	}
}
