package cli

import (
	"io"
	"reflect"
	"testing"
)

func TestExtractGlobalFlagsKeepsCommandFlags(t *testing.T) {
	globals, args, err := extractGlobalFlags([]string{
		"companies",
		"list",
		"--format",
		"json",
		"--tickers",
		"AAPL",
		"--api-key=secret",
		"--debug",
	})
	if err != nil {
		t.Fatal(err)
	}

	if globals.Format == nil || *globals.Format != "json" {
		t.Fatalf("expected global format json, got %#v", globals.Format)
	}
	if globals.APIKey == nil || *globals.APIKey != "secret" {
		t.Fatalf("expected global API key secret, got %#v", globals.APIKey)
	}
	if !globals.Debug {
		t.Fatal("expected debug global to be set")
	}

	want := []string{"companies", "list", "--tickers", "AAPL"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args: expected %#v, got %#v", want, args)
	}
}

func TestParseInterspersedAllowsFlagsAfterPositionals(t *testing.T) {
	fs := newFlagSet("download", io.Discard)
	output := fs.String("output", "", "output file")
	withAPIKey := fs.Bool("with-api-key", false, "include key")

	err := parseInterspersed(fs, []string{"abc", "--output", "file.json", "--with-api-key"})
	if err != nil {
		t.Fatal(err)
	}

	if fs.NArg() != 1 || fs.Arg(0) != "abc" {
		t.Fatalf("positionals: expected [abc], got %#v", fs.Args())
	}
	if *output != "file.json" {
		t.Fatalf("output: expected file.json, got %q", *output)
	}
	if !*withAPIKey {
		t.Fatal("expected with-api-key to be set")
	}
}

func TestListFlagsMapCompanyIDsPerEndpoint(t *testing.T) {
	companies, ok := resourceByName("companies")
	if !ok {
		t.Fatal("companies resource not found")
	}
	companyParams := listFlags{limit: 10, companyIDs: "4742"}.toParams(companies.listParams, companies.name == "companies")
	if got := companyParams.Get("ids"); got != "4742" {
		t.Fatalf("companies ids: expected 4742, got %q", got)
	}
	if got := companyParams.Get("companyIds"); got != "" {
		t.Fatalf("companies companyIds: expected empty, got %q", got)
	}

	// --company-ids and --ids feed the same parameter on this endpoint, so
	// passing both has to merge rather than let one overwrite the other.
	merged := listFlags{limit: 10, companyIDs: "4742", ids: "3694,4742"}.toParams(companies.listParams, true)
	if got := merged.Get("ids"); got != "4742,3694" {
		t.Fatalf("companies ids: expected merged 4742,3694, got %q", got)
	}

	events, ok := resourceByName("events")
	if !ok {
		t.Fatal("events resource not found")
	}
	eventParams := listFlags{limit: 10, companyIDs: "4742"}.toParams(events.listParams, events.name == "companies")
	if got := eventParams.Get("companyIds"); got != "4742" {
		t.Fatalf("events companyIds: expected 4742, got %q", got)
	}
	if got := eventParams.Get("ids"); got != "" {
		t.Fatalf("events ids: expected empty, got %q", got)
	}
}
