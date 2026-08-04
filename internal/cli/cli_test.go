package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompaniesListSendsAPIKeyAndFormatsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/companies" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Fatalf("expected x-api-key secret, got %q", got)
		}
		if got := r.URL.Query().Get("tickers"); got != "AAPL" {
			t.Fatalf("expected tickers=AAPL, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":4742,"name":"Apple Inc."}],"pagination":{"nextCursor":null}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json", "companies", "list", "--tickers", "AAPL"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Apple Inc.") {
		t.Fatalf("expected output to contain Apple Inc., got %s", out.String())
	}
}

func TestDownloadAllowsFlagsAfterID(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/documents/transcripts/abc":
			if got := r.Header.Get("x-api-key"); got != "secret" {
				t.Fatalf("expected x-api-key secret, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"abc","fileUrl":"` + srv.URL + `/file/transcript.json"}}`))
		case "/file/transcript.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "transcript.json")
	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "transcripts", "download", "abc", "--output", dest}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"ok":true}` {
		t.Fatalf("unexpected downloaded body: %s", string(b))
	}
}

func TestSortByRejectedOnUnsortableResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no request, got %s", r.URL)
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"transcripts", "list", "--tickers", "AAPL", "--sort-by", "date"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d; stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"--sort-by is not supported", "quartr events list", "--event-ids"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("expected stderr to mention %q, got %s", want, errOut.String())
		}
	}
}

func TestSortByRejectsUnknownFieldOnEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no request, got %s", r.URL)
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"events", "list", "--sort-by", "title"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "supported sort fields: id, date") {
		t.Fatalf("expected supported field list, got %s", errOut.String())
	}
}

func TestSortByForwardedOnEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sortBy"); got != "date" {
			t.Fatalf("expected sortBy=date, got %q", got)
		}
		if got := r.URL.Query().Get("direction"); got != "desc" {
			t.Fatalf("expected direction=desc, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"Q4 2025"}],"pagination":{"nextCursor":null}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"events", "list", "--sort-by", "date", "--direction", "desc"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
}

func TestSortByRejectedOnChildList(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", "http://127.0.0.1:0",
		"reports", "pages", "123", "--sort-by", "date"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "quartr reports pages") {
		t.Fatalf("expected command name in message, got %s", errOut.String())
	}
}

func TestExpandCompanyJoinsNamesClientSide(t *testing.T) {
	var eventQueries, companyQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/events":
			eventQueries = append(eventQueries, r.URL.RawQuery)
			_, _ = w.Write([]byte(`{"data":[
				{"id":1,"title":"Q4 2025","companyId":3694},
				{"id":2,"title":"Q1 2026","companyId":12301},
				{"id":3,"title":"Q2 2026","companyId":3694}
			],"pagination":{"nextCursor":null}}`))
		case "/companies":
			companyQueries = append(companyQueries, r.URL.Query().Get("ids"))
			_, _ = w.Write([]byte(`{"data":[
				{"id":3694,"name":"Arcosa Inc","country":"US"},
				{"id":12301,"name":"Crédit Agricole S.A.","country":"FR"}
			],"pagination":{"nextCursor":null}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json",
		"events", "list", "--tickers", "ACA", "--expand", "company"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"Arcosa Inc", "Crédit Agricole S.A."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected joined company %q in output, got %s", want, out.String())
		}
	}
	// expand=company must not reach the API: /events 400s on the parameter.
	if len(eventQueries) != 1 || strings.Contains(eventQueries[0], "expand") {
		t.Fatalf("expected one events request without expand, got %#v", eventQueries)
	}
	// Two rows share a companyId, so the join asks for two distinct ids once.
	if len(companyQueries) != 1 {
		t.Fatalf("expected exactly 1 companies request, got %#v", companyQueries)
	}
	if companyQueries[0] != "3694,12301" {
		t.Fatalf("expected deduped ids 3694,12301, got %q", companyQueries[0])
	}
}

func TestExpandCompanyKeepsEventExpansionOnTheWire(t *testing.T) {
	var gotExpand string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/documents/transcripts":
			gotExpand = r.URL.Query().Get("expand")
			_, _ = w.Write([]byte(`{"data":[{"id":9,"companyId":4742,"event":{"title":"Q3 2026"}}],"pagination":{"nextCursor":null}}`))
		case "/companies":
			_, _ = w.Write([]byte(`{"data":[{"id":4742,"name":"Apple Inc"}],"pagination":{"nextCursor":null}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json",
		"transcripts", "list", "--expand", "event,company"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if gotExpand != "event" {
		t.Fatalf("expected expand=event forwarded without company, got %q", gotExpand)
	}
	if !strings.Contains(out.String(), "Apple Inc") {
		t.Fatalf("expected joined company name, got %s", out.String())
	}
}

func TestExpandCompanyRejectedWhereRowsHaveNoCompany(t *testing.T) {
	for _, tc := range []struct{ cmd, want string }{
		{"companies", "redundant"},
		{"document-types", "no companyId"},
	} {
		var out, errOut bytes.Buffer
		code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", "http://127.0.0.1:0",
			tc.cmd, "list", "--expand", "company"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("%s: expected usage exit code 2, got %d; stderr=%s", tc.cmd, code, errOut.String())
		}
		if !strings.Contains(errOut.String(), tc.want) {
			t.Fatalf("%s: expected stderr to mention %q, got %s", tc.cmd, tc.want, errOut.String())
		}
	}
}

func TestExpandCompanyFailureIsNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/companies" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Forbidden","statusCode":403}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"Q4 2025","companyId":3694}],"pagination":{"nextCursor":null}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json",
		"events", "list", "--expand", "company"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected the rows to survive a failed join, got code %d", code)
	}
	if !strings.Contains(out.String(), "Q4 2025") {
		t.Fatalf("expected event rows in stdout, got %s", out.String())
	}
	if !strings.Contains(errOut.String(), "warning: --expand company") {
		t.Fatalf("expected a warning on stderr, got %s", errOut.String())
	}
}

// twoBLDCompanies serves the TopBuild / Boral ticker collision from #5.
func twoBLDCompanies() string {
	return `{"data":[
		{"id":11909,"name":"TopBuild Corp","country":"US","tickers":[{"exchange":"NYSE","ticker":"BLD"}]},
		{"id":14573,"name":"Boral Limited","country":"AU","tickers":[{"exchange":"ASX","ticker":"BLD"}]}
	],"pagination":{"nextCursor":null}}`
}

func TestQualifiedTickerResolvesToCompanyIDs(t *testing.T) {
	var eventQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/companies":
			if got := r.URL.Query().Get("tickers"); got != "BLD" {
				t.Errorf("expected bare ticker BLD on the wire, got %q", got)
			}
			_, _ = w.Write([]byte(twoBLDCompanies()))
		case "/events":
			eventQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"Q4 2025","companyId":11909}],"pagination":{"nextCursor":null}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json",
		"events", "list", "--tickers", "NYSE:BLD"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	// Resolving to an id beats filtering the page: rows belonging to the
	// other company never consume the caller's --limit.
	if got := eventQuery.Get("companyIds"); got != "11909" {
		t.Fatalf("expected companyIds=11909, got %q", got)
	}
	if got := eventQuery.Get("tickers"); got != "" {
		t.Fatalf("expected tickers to be replaced, got %q", got)
	}
}

func TestUnqualifiedTickerIsNotResolved(t *testing.T) {
	var tickers string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Errorf("expected no company lookup, got %s", r.URL.Path)
		}
		tickers = r.URL.Query().Get("tickers")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"events", "list", "--tickers", "AAPL,aapl,MSFT,AAPL"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if tickers != "AAPL,MSFT" {
		t.Fatalf("expected case-duplicates collapsed to AAPL,MSFT, got %q", tickers)
	}
}

func TestQualifiedTickerWithNoMatchIsAUsageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/companies" {
			t.Errorf("expected no list request, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoBLDCompanies()))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"events", "list", "--tickers", "NASDAQ:BLD"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "companies resolve BLD") {
		t.Fatalf("expected a pointer to `companies resolve`, got %s", errOut.String())
	}
}

func TestCompaniesResolveListsEveryCandidate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/companies" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoBLDCompanies()))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"companies", "resolve", "BLD"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"TopBuild Corp", "NYSE:BLD", "Boral Limited", "ASX:BLD"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected %q in resolve output, got %s", want, out.String())
		}
	}
}

func TestCompaniesResolveRejectsNameSearch(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", "http://127.0.0.1:0",
		"companies", "resolve", "Apple Inc"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %d; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "no company name search") {
		t.Fatalf("expected an explanation of the missing capability, got %s", errOut.String())
	}
}

// downloadServer serves one document plus its metadata record.
func downloadServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/documents/transcripts/abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"abc","fileUrl":"` + srv.URL + `/file/transcript.json"}}`))
		case "/file/transcript.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	return srv
}

func TestDownloadToStdout(t *testing.T) {
	const body = `{"transcript":"hello"}`
	srv := downloadServer(t, body)
	defer srv.Close()

	dir := t.TempDir()
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"transcripts", "download", "abc", "--output", "-"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	// stdout is the document, byte for byte — a redirect must capture this
	// and nothing else.
	if out.String() != body {
		t.Fatalf("expected the document on stdout, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected nothing on stderr, got %q", errOut.String())
	}
	// And no stray file is left behind next to the redirect target.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written, got %v", entries)
	}
}

func TestDownloadStatusLineGoesToStderr(t *testing.T) {
	srv := downloadServer(t, `{"ok":true}`)
	defer srv.Close()

	t.Chdir(t.TempDir())

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"transcripts", "download", "abc"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected clean stdout, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "Saved transcripts-abc.json") {
		t.Fatalf("expected the saved path on stderr, got %q", errOut.String())
	}
	if _, err := os.Stat("transcripts-abc.json"); err != nil {
		t.Fatalf("expected the default file to exist: %v", err)
	}
}

func TestForbiddenIsExplainedAsTierNotAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Forbidden","statusCode":403}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"events", "summary", "406161"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "not included in your API tier") {
		t.Fatalf("expected a tier explanation, got %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "401") {
		t.Fatalf("expected the 401 contrast that rules out a bad key, got %s", errOut.String())
	}
}

func TestUnauthorizedPointsAtTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized","statusCode":401}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"companies", "list"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "quartr auth show") {
		t.Fatalf("expected the key-checking hint, got %s", errOut.String())
	}
}

func TestLookupTablesReturnTheWholeCatalog(t *testing.T) {
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Errorf("expected the catalog to be fetched 500 at a time, got limit=%q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":11,"name":"Annual report","form":"10-K"}],"pagination":{"nextCursor":"p2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":25,"name":"Shareholder letter","form":""}],"pagination":{"nextCursor":null}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"document-types", "list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if pages != 2 {
		t.Fatalf("expected the catalog to be paged to exhaustion, got %d requests", pages)
	}
	// typeId 25 lives past the old default page of 10, which is why it read
	// as undocumented.
	if !strings.Contains(out.String(), "Shareholder letter") {
		t.Fatalf("expected the tail of the catalog, got %s", out.String())
	}
}

func TestExplicitLimitStillPagesTheCatalog(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("expected limit=5 to be honored, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":11,"name":"Annual report"}],"pagination":{"nextCursor":"p2"}}`))
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL,
		"document-types", "list", "--limit", "5"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if requests != 1 {
		t.Fatalf("expected an explicit --limit to opt out of the full catalog, got %d requests", requests)
	}
}

func TestListAllFollowsPagination(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("cursor") {
		case "":
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Fatalf("expected default --all limit 500, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":1,"title":"First"}],"pagination":{"nextCursor":"next"}}`))
		case "next":
			_, _ = w.Write([]byte(`{"data":[{"id":2,"title":"Second"}],"pagination":{"nextCursor":null}}`))
		default:
			t.Fatalf("unexpected cursor: %s", r.URL.Query().Get("cursor"))
		}
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"--no-config", "--api-key", "secret", "--base-url", srv.URL, "--format", "json", "events", "list", "--all"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%s", code, errOut.String())
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
	if !strings.Contains(out.String(), `"count": 2`) {
		t.Fatalf("expected output count 2, got %s", out.String())
	}
	if !strings.Contains(out.String(), "First") || !strings.Contains(out.String(), "Second") {
		t.Fatalf("expected both rows, got %s", out.String())
	}
}
