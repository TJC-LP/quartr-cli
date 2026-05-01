package quartr

import (
	"net/url"
	"testing"
	"time"
)

func TestBuildURLAddsParamsAndSkipsEmptyValues(t *testing.T) {
	c := NewClient("https://api.example/base/", "key", time.Second, false)
	params := url.Values{
		"tickers": {"AAPL"},
		"empty":   {""},
	}

	got, err := c.BuildURL("/companies?limit=10", params)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://api.example/base/companies?limit=10&tickers=AAPL"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildURLAcceptsAbsoluteURL(t *testing.T) {
	c := NewClient("https://api.example", "key", time.Second, false)
	got, err := c.BuildURL("https://files.example/report.pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://files.example/report.pdf" {
		t.Fatalf("expected absolute URL to pass through, got %q", got)
	}
}
