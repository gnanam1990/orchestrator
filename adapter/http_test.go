package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gnanam1990/orchestrator/catalog"
)

// delegate builds a minimal delegate manifest pointing at invoke.
func delegate(invoke string) catalog.Manifest {
	return catalog.Manifest{
		Name:        "test-entry",
		Type:        catalog.TypeDelegate,
		Adapter:     catalog.AdapterCompliant,
		Invoke:      invoke,
		Description: "a test entry",
		Permission:  catalog.PermissionAuto,
	}
}

// TestHTTPAdapter_Success200 checks a 200 with a JSON body flows back intact.
func TestHTTPAdapter_Success200(t *testing.T) {
	const body = `{"ok":true,"temperature_2m":21.3}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	got, err := NewHTTPAdapter().Invoke(context.Background(), delegate(srv.URL), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Output != body {
		t.Errorf("Output = %q, want %q", got.Output, body)
	}
	if got.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", got.StatusCode)
	}
	if got.Duration <= 0 {
		t.Errorf("Duration should be positive; got %v", got.Duration)
	}
}

// TestHTTPAdapter_Non2xxIsError checks a 500 becomes an error, with the Result
// still populated (not silently empty).
func TestHTTPAdapter_Non2xxIsError(t *testing.T) {
	const body = `{"error":"boom"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	got, err := NewHTTPAdapter().Invoke(context.Background(), delegate(srv.URL), "")
	if err == nil {
		t.Fatalf("expected an error for a 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention the status code; got: %v", err)
	}
	// "never silently accept a bad outcome": the diagnostic metadata survives.
	if got.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected StatusCode 500 on the error path; got %d", got.StatusCode)
	}
	if got.Output != body {
		t.Errorf("expected the response body carried on the error path; got %q", got.Output)
	}
}

// TestHTTPAdapter_Timeout checks a slow endpoint is bounded by the context
// deadline rather than hanging.
func TestHTTPAdapter_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond only after a long delay, but abandon promptly when the client
		// cancels so the test (and server shutdown) stay fast.
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	a := NewHTTPAdapter()
	a.Timeout = 50 * time.Millisecond
	_, err := a.Invoke(context.Background(), delegate(srv.URL), "")
	if err == nil {
		t.Fatalf("expected a timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded; got: %v", err)
	}
}

// TestHTTPAdapter_TaskSubstitution checks {task} is URL-escaped and substituted.
func TestHTTPAdapter_TaskSubstitution(t *testing.T) {
	const task = "berlin weather & sun"
	var gotDecoded, gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDecoded = r.URL.Query().Get("q") // server decodes the escaped value
		gotRaw = r.URL.RawQuery
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	entry := delegate(srv.URL + "?q={task}")
	if _, err := NewHTTPAdapter().Invoke(context.Background(), entry, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The escaped value round-trips back to the original task on the server.
	if gotDecoded != task {
		t.Errorf("decoded task = %q, want %q", gotDecoded, task)
	}
	// The placeholder is gone and the value is escaped: no literal placeholder,
	// no raw space, and the task's '&' did not leak as a query separator.
	if strings.Contains(gotRaw, "{task}") {
		t.Errorf("placeholder was not substituted; raw query = %q", gotRaw)
	}
	if strings.Contains(gotRaw, " ") {
		t.Errorf("task was not URL-escaped (contains a raw space); raw query = %q", gotRaw)
	}
}

// TestHTTPAdapter_NoPlaceholderSkipsSubstitution checks the URL is used as-is
// (and the task ignored) when {task} is absent.
func TestHTTPAdapter_NoPlaceholderSkipsSubstitution(t *testing.T) {
	var gotPath, gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRaw = r.URL.RawQuery
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	entry := delegate(srv.URL + "/forecast?lat=52.52")
	got, err := NewHTTPAdapter().Invoke(context.Background(), entry, "this task must be ignored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/forecast" {
		t.Errorf("path = %q, want /forecast", gotPath)
	}
	if gotRaw != "lat=52.52" {
		t.Errorf("query = %q, want lat=52.52 (task must not be appended)", gotRaw)
	}
	if got.Output != "ok" {
		t.Errorf("Output = %q, want ok", got.Output)
	}
}

// TestHTTPAdapter_EmptyInvokeErrors checks an entry with no URL is rejected.
func TestHTTPAdapter_EmptyInvokeErrors(t *testing.T) {
	if _, err := NewHTTPAdapter().Invoke(context.Background(), delegate(""), "task"); err == nil {
		t.Fatalf("expected an error for an empty invoke URL, got nil")
	}
}

// TestBuildURL pins the templating/escaping behavior directly.
func TestBuildURL(t *testing.T) {
	cases := []struct{ name, invoke, task, want string }{
		{"no placeholder", "https://example.test/forecast?lat=52.52", "ignored", "https://example.test/forecast?lat=52.52"},
		{"escaped value", "https://example.test/s?q={task}", "a b&c", "https://example.test/s?q=a+b%26c"},
		{"multiple occurrences", "https://example.test/s?a={task}&b={task}", "x y", "https://example.test/s?a=x+y&b=x+y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildURL(tc.invoke, tc.task); got != tc.want {
				t.Errorf("buildURL(%q, %q) = %q, want %q", tc.invoke, tc.task, got, tc.want)
			}
		})
	}
}
