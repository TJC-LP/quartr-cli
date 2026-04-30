package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
