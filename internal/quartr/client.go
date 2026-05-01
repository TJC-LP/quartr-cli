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

const (
	DefaultBaseURL = "https://api.quartr.com/public/v3"
	UserAgent      = "quartr-cli/0.1.0"
)

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Debug   bool
}

type APIError struct {
	StatusCode int
	Status     string
	Body       string
	Headers    http.Header
}

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
