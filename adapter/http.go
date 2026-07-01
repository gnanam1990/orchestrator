package adapter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gnanam1990/orchestrator/catalog"
)

// taskPlaceholder is the templating token shared with the CLI manifest
// convention: wherever it appears in an invoke string, the URL-escaped task is
// substituted in.
const taskPlaceholder = "{task}"

// defaultTimeout bounds a single invocation so a slow or dead endpoint cannot
// hang the adapter forever. The caller's context deadline still applies and
// takes effect if it is sooner.
const defaultTimeout = 30 * time.Second

// HTTPAdapter is the compliant adapter for HTTP endpoints: it issues a GET to
// entry.Invoke (after {task} substitution) and returns the response body.
type HTTPAdapter struct {
	// Client performs the request; nil means http.DefaultClient.
	Client *http.Client
	// Timeout bounds each invocation via a context deadline; <= 0 disables the
	// adapter's own bound and relies solely on the caller's context.
	Timeout time.Duration
}

// Compile-time check that HTTPAdapter satisfies Adapter.
var _ Adapter = (*HTTPAdapter)(nil)

// NewHTTPAdapter returns an HTTPAdapter with a default client and timeout.
func NewHTTPAdapter() *HTTPAdapter {
	return &HTTPAdapter{
		Client:  &http.Client{},
		Timeout: defaultTimeout,
	}
}

// Invoke issues a GET to entry.Invoke and returns the response body.
//
// If entry.Invoke contains the literal "{task}", every occurrence is replaced
// with the URL-escaped task before the request; otherwise the URL is used
// as-is. The request is bounded by ctx (and by the adapter's Timeout, if set),
// so a slow endpoint times out rather than hanging. A non-2xx response is
// returned as an error with the populated Result attached — a bad outcome is
// never silently accepted.
func (a *HTTPAdapter) Invoke(ctx context.Context, entry catalog.Manifest, task string) (Result, error) {
	if entry.Invoke == "" {
		return Result{}, fmt.Errorf("adapter: entry %q has no invoke URL", entry.Name)
	}
	target := buildURL(entry.Invoke, task)

	if a.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Result{}, fmt.Errorf("adapter: building request for %q: %w", target, err)
	}

	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("adapter: GET %q failed: %w", target, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return Result{}, fmt.Errorf("adapter: reading response from %q: %w", target, err)
	}

	result := Result{
		Output:     string(body),
		StatusCode: resp.StatusCode,
		Duration:   duration,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("adapter: GET %q returned non-2xx status %d", target, resp.StatusCode)
	}
	return result, nil
}

// buildURL applies the {task} templating convention: if invoke contains the
// placeholder, every occurrence is replaced with the URL-escaped task;
// otherwise invoke is returned unchanged.
//
// Escaping is url.QueryEscape (query-value rules: space -> '+', '&' and '/'
// percent-encoded), matching the query-parameter convention these manifests use
// (e.g. ?q={task}). This is deliberately not url.PathEscape: PathEscape leaves
// '&' unescaped, which would corrupt a query value. If a future manifest
// templates {task} into a path segment instead, revisit the escaping here.
func buildURL(invoke, task string) string {
	if !strings.Contains(invoke, taskPlaceholder) {
		return invoke
	}
	return strings.ReplaceAll(invoke, taskPlaceholder, url.QueryEscape(task))
}
