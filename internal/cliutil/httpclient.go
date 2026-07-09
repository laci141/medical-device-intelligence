package cliutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError carries the HTTP status and a short body snippet for diagnosis.
// Callers inspect StatusCode: a 404 is "no records" (data, exit 0), not a
// failure. See guardrail 5.
type APIError struct {
	StatusCode int
	Snippet    string // ~200 bytes of the response body
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("http %d for %s: %s", e.StatusCode, e.URL, e.Snippet)
}

// Client is a small keyless JSON HTTP client with a single retry policy.
type Client struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
	// Backoff is the pause before the single 5xx retry; small by default.
	Backoff time.Duration
	// MaxBodyBytes caps the response body. openFDA records are large (a single
	// device enforcement record is ~66 KB, so a page of 50 is ~3.3 MB and a full
	// page of 1000 approaches ~66 MB), so this must be generous. If a response
	// exceeds it, GetJSON returns a CLEAR error rather than silently truncating
	// into an "unexpected end of JSON input" — the exact bug a 1 MB cap caused.
	MaxBodyBytes int64
}

const defaultMaxBodyBytes = 128 << 20 // 128 MiB — comfortably fits a full openFDA page

// NewClient returns a Client with sane keyless defaults.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		UserAgent:    "medical-device-intelligence-pp-cli/0.1 (keyless)",
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		Backoff:      300 * time.Millisecond,
		MaxBodyBytes: defaultMaxBodyBytes,
	}
}

// GetJSON issues GET BaseURL+path?params and returns the raw body and status.
//
// Retry policy (guardrail 6): retry exactly once, only on a 5xx, after a short
// backoff. A 4xx is returned immediately — never retried. Any non-2xx yields an
// *APIError carrying a ~200-byte body snippet. A 404 is returned as an
// *APIError with StatusCode 404 so the caller can treat it as "no records".
func (c *Client) GetJSON(ctx context.Context, path string, params url.Values) ([]byte, int, error) {
	u := c.BaseURL + path
	if len(params) > 0 {
		// url.Values.Encode turns spaces into "+" — exactly what the Lucene
		// builders rely on. Never hand-encode the search expression.
		u += "?" + params.Encode()
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(c.Backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue // transport error — allow the single retry
		}
		max := c.MaxBodyBytes
		if max <= 0 {
			max = defaultMaxBodyBytes
		}
		// Read one byte past the cap so we can detect (rather than silently
		// swallow) a body that exceeds it.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, max+1))
		resp.Body.Close()
		if int64(len(body)) > max {
			return nil, resp.StatusCode, &APIError{
				StatusCode: resp.StatusCode,
				Snippet:    fmt.Sprintf("response body exceeded %d-byte cap; lower --limit", max),
				URL:        u,
			}
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, resp.StatusCode, nil
		case resp.StatusCode >= 500:
			lastErr = &APIError{StatusCode: resp.StatusCode, Snippet: snippet(body), URL: u}
			continue // retry once on 5xx
		default: // 4xx (incl. 404) — return immediately, never retried
			return body, resp.StatusCode, &APIError{StatusCode: resp.StatusCode, Snippet: snippet(body), URL: u}
		}
	}
	return nil, 0, lastErr
}

func snippet(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
