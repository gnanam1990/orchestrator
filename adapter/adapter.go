// Package adapter defines how the orchestrator invokes a selected catalog
// entry, plus a compliant HTTP adapter (GET-based invocation).
//
// This is step 3: the HTTP-invocation shape only. Other adapters (CLI
// subprocess, …) and the executor/reporter are intentionally not built here.
package adapter

import (
	"context"
	"time"

	"github.com/gnanam1990/orchestrator/catalog"
)

// Result is the outcome of invoking a catalog entry.
type Result struct {
	Output     string        // raw response body
	StatusCode int           // HTTP status code of the response
	Duration   time.Duration // wall-clock time taken by the invocation
}

// Adapter invokes a catalog entry for a given task. Concrete adapters (HTTP,
// CLI subprocess, …) implement this so a future executor can treat them
// uniformly.
type Adapter interface {
	Invoke(ctx context.Context, entry catalog.Manifest, task string) (Result, error)
}
