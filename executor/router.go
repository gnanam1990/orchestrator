package executor

import (
	"fmt"
	"strings"

	"github.com/gnanam1990/orchestrator/adapter"
	"github.com/gnanam1990/orchestrator/catalog"
)

// Route maps a catalog entry to the adapter that can invoke it, based on the
// entry's adapter kind and the shape of its invoke string.
//
// For now only the "compliant" kind with an http(s) invoke URL is implemented
// (HTTPAdapter). Any other invocation shape — e.g. the CLI-subprocess shape
// (step 6) — returns a clear "not implemented yet" error rather than a wrong
// guess, so a mis-shaped entry is never silently misrouted.
func Route(entry catalog.Manifest) (adapter.Adapter, error) {
	switch entry.Adapter {
	case catalog.AdapterCompliant:
		if strings.HasPrefix(entry.Invoke, "http://") || strings.HasPrefix(entry.Invoke, "https://") {
			return adapter.NewHTTPAdapter(), nil
		}
		return nil, fmt.Errorf("no adapter implemented for the invocation shape of entry %q (invoke: %q) yet", entry.Name, entry.Invoke)
	default:
		return nil, fmt.Errorf("adapter kind %q for entry %q is not implemented yet", entry.Adapter, entry.Name)
	}
}
