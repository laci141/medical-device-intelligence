package cliutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(base string) *Client {
	c := NewClient(base)
	c.Backoff = 1 // keep retries fast in tests
	return c
}

// TestLargeBodyUnderCapParses is the regression for the 1 MB truncation bug: a
// body larger than the OLD 1 MB cap must be returned whole, not truncated into
// invalid JSON.
func TestLargeBodyUnderCapParses(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", 3<<20) + `"}` // ~3 MB, valid JSON
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(big))
	}))
	defer srv.Close()

	body, status, err := testClient(srv.URL).GetJSON(context.Background(), "/x", nil)
	if err != nil {
		t.Fatalf("large body must not error: %v", err)
	}
	if status != 200 || len(body) != len(big) {
		t.Fatalf("body truncated: got %d bytes want %d", len(body), len(big))
	}
}

// TestBodyOverCapErrorsClearly proves an over-cap body yields a clear error, not
// a cryptic downstream JSON failure.
func TestBodyOverCapErrorsClearly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("y", 5000)))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	c.MaxBodyBytes = 1000 // force truncation
	_, _, err := c.GetJSON(context.Background(), "/x", nil)
	if err == nil {
		t.Fatal("over-cap body must return an error")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("error should explain the cap, got %v", err)
	}
}

// TestRetryOnceOn5xxThenSucceed proves guardrail 6: exactly one retry on 5xx.
func TestRetryOnceOn5xxThenSucceed(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	body, status, err := testClient(srv.URL).GetJSON(context.Background(), "/x", nil)
	if err != nil {
		t.Fatalf("should succeed on retry: %v", err)
	}
	if status != 200 || calls != 2 {
		t.Fatalf("expected 2 calls (1 fail + 1 retry), got calls=%d status=%d", calls, status)
	}
	if !strings.Contains(string(body), "ok") {
		t.Errorf("unexpected body %q", body)
	}
}

// TestNoRetryOn4xx proves a 4xx is returned immediately, never retried.
func TestNoRetryOn4xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"NOT_FOUND"}`))
	}))
	defer srv.Close()

	_, status, err := testClient(srv.URL).GetJSON(context.Background(), "/x", nil)
	if calls != 1 {
		t.Fatalf("4xx must not be retried; calls=%d", calls)
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != 404 || status != 404 {
		t.Fatalf("expected *APIError 404, got status=%d err=%v", status, err)
	}
}
