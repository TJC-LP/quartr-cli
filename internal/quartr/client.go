// Package quartr is the HTTP client and on-disk config layer for the Quartr
// Public API v3. It depends only on the Go standard library.
package quartr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultBaseURL is the production API endpoint used when no override is configured.
// Version and UserAgent live in version.go.
const DefaultBaseURL = "https://api.quartr.com/public/v3"

// Client performs authenticated GET requests against the Quartr Public API.
// It retries 429 and 5xx responses with backoff and honors Retry-After.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Debug   bool
}

// APIError is returned when the API responds with a non-2xx status.
// The full response Body is preserved (truncated at 800 chars in Error())
// so callers can inspect the structured error payload.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
	Headers    http.Header
}

// Error implements the error interface, returning a one-line summary
// suitable for printing to a terminal.
func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 800 {
		body = body[:800] + "…"
	}
	if body == "" {
		return "quartr api error: " + e.Status
	}
	return fmt.Sprintf("quartr api error: %s: %s", e.Status, body)
}

// NewClient constructs a Client. An empty baseURL falls back to DefaultBaseURL.
// A non-positive timeout is replaced with 30 seconds.
func NewClient(baseURL, apiKey string, timeout time.Duration, debug bool) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  strings.TrimSpace(apiKey),
		HTTP:    &http.Client{Timeout: timeout},
		Debug:   debug,
	}
}

// GetBytes performs a GET against BaseURL+path with params as the query
// string. The API key is sent in the x-api-key header. Retries on 429 and
// 5xx with exponential backoff (250ms, 500ms) and Retry-After honoring.
// Returns the raw body, response headers, and any error.
func (c *Client) GetBytes(ctx context.Context, path string, params url.Values) ([]byte, http.Header, error) {
	if c.APIKey == "" {
		return nil, nil, errors.New("missing API key; set QUARTR_API_KEY or run `quartr auth login --api-key ...`")
	}
	u, err := c.BuildURL(path, params)
	if err != nil {
		return nil, nil, err
	}
	if c.Debug {
		fmt.Fprintf(os.Stderr, "GET %s\n", u)
	}

	var lastHeader http.Header
	var lastErr error
	for attempt := range 3 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("x-api-key", c.APIKey)
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				if sleep(ctx, backoff(attempt)) != nil {
					return nil, nil, err
				}
				continue
			}
			return nil, nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lastHeader = resp.Header
		if readErr != nil {
			return nil, resp.Header, readErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, resp.Header, nil
		}
		lastErr = &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(body),
			Headers:    resp.Header,
		}
		if attempt < 2 && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			d := retryAfter(resp.Header)
			if d <= 0 {
				d = backoff(attempt)
			}
			if err := sleep(ctx, d); err != nil {
				return nil, resp.Header, err
			}
			continue
		}
		return nil, resp.Header, lastErr
	}
	return nil, lastHeader, lastErr
}

func backoff(attempt int) time.Duration {
	return time.Duration(250*(1<<attempt)) * time.Millisecond
}

func retryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(v + "s"); err == nil {
		return seconds
	}
	if when, err := http.ParseTime(v); err == nil {
		return time.Until(when)
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// GetJSON wraps GetBytes and decodes the response into a generic
// map[string]any. Numbers are preserved as json.Number to avoid float
// coercion of int64-shaped IDs.
func (c *Client) GetJSON(ctx context.Context, path string, params url.Values) (map[string]any, http.Header, error) {
	body, hdr, err := c.GetBytes(ctx, path, params)
	if err != nil {
		return nil, hdr, err
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, hdr, fmt.Errorf("decode json response: %w", err)
	}
	return out, hdr, nil
}

// BuildURL composes the request URL from BaseURL + path + params. If path
// is itself a fully qualified URL it is used verbatim, allowing callers to
// pass redirect targets returned by the API.
func (c *Client) BuildURL(path string, params url.Values) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	var u *url.URL
	var err error
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		u, err = url.Parse(path)
	} else {
		base := strings.TrimRight(c.BaseURL, "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u, err = url.Parse(base + path)
	}
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, vals := range params {
		for _, v := range vals {
			if v != "" {
				q.Add(k, v)
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Download fetches rawURL and streams the body to w. apiKey is sent in the
// x-api-key header only when non-empty; most Quartr file URLs are public,
// so callers typically pass "".
func (c *Client) Download(ctx context.Context, rawURL, apiKey string, w io.Writer) (http.Header, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("empty download url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.Header, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body), Headers: resp.Header}
	}
	_, err = io.Copy(w, resp.Body)
	return resp.Header, err
}
