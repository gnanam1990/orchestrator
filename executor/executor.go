// Package executor runs the orchestrator's decision pipeline: select an entry,
// apply its permission policy, route it to an adapter, and invoke it.
//
// This is step 4: the executor and permission engine. The reporter, verifier,
// self-check, and CLI-subprocess adapter are intentionally not built here.
package executor

import (
	"context"
	"fmt"

	"github.com/gnanam1990/orchestrator/adapter"
	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/selector"
)

// Approver decides whether an entry whose permission is "ask" may run. Keeping
// it an interface (mirroring selector.LLMCaller) lets tests approve or deny
// without real stdin.
type Approver interface {
	Approve(ctx context.Context, entry catalog.Manifest, task string) (bool, error)
}

// Decision classifies the control-flow outcome of Execute — the expected
// results that are NOT errors, so a caller can tell "nothing matched" or "user
// declined" apart from a real failure (which comes back as a non-nil error).
type Decision int

const (
	// DecisionNoMatch means selection found no suitable entry.
	DecisionNoMatch Decision = iota
	// DecisionRejectedNever means the entry's permission is "never".
	DecisionRejectedNever
	// DecisionRejectedByApprover means permission was "ask" and the approver declined.
	DecisionRejectedByApprover
	// DecisionInvoked means the entry was routed to an adapter and invoked.
	DecisionInvoked
)

func (d Decision) String() string {
	switch d {
	case DecisionNoMatch:
		return "no-match"
	case DecisionRejectedNever:
		return "rejected (never)"
	case DecisionRejectedByApprover:
		return "rejected (declined)"
	case DecisionInvoked:
		return "invoked"
	default:
		return fmt.Sprintf("Decision(%d)", int(d))
	}
}

// Outcome is the result of Execute. Decision classifies what happened; Entry is
// the selected entry (zero on no-match); Result is the adapter result and is
// meaningful only when Decision == DecisionInvoked.
type Outcome struct {
	Decision Decision
	Entry    catalog.Manifest
	Result   adapter.Result
}

// Executor runs the pipeline. Route maps an entry to its adapter; it is a field
// so tests can inject a spy adapter. A zero-value Executor uses the default
// Route.
type Executor struct {
	Route func(entry catalog.Manifest) (adapter.Adapter, error)
}

// New returns an Executor using the default adapter router.
func New() *Executor {
	return &Executor{Route: Route}
}

// Execute selects an entry for task, applies its permission policy, and — if
// permitted — routes it to an adapter and invokes it.
//
// No-match and rejections (never / approver-declined) come back as an Outcome
// with a nil error: they are expected outcomes, distinguishable from real
// failures (selection error, routing error, invocation error), which come back
// as a non-nil error.
func (e *Executor) Execute(ctx context.Context, task string, entries []catalog.Manifest, sel *selector.Selector, appr Approver) (Outcome, error) {
	sr, err := sel.Select(ctx, task, entries)
	if err != nil {
		return Outcome{}, fmt.Errorf("executor: selection failed: %w", err)
	}
	if !sr.Matched {
		return Outcome{Decision: DecisionNoMatch}, nil
	}
	entry := sr.Entry

	switch entry.Permission {
	case catalog.PermissionNever:
		// "never" is rejected immediately — never routed or invoked.
		return Outcome{Decision: DecisionRejectedNever, Entry: entry}, nil
	case catalog.PermissionAsk:
		ok, err := appr.Approve(ctx, entry, task)
		if err != nil {
			return Outcome{}, fmt.Errorf("executor: approval for %q failed: %w", entry.Name, err)
		}
		if !ok {
			return Outcome{Decision: DecisionRejectedByApprover, Entry: entry}, nil
		}
	case catalog.PermissionAuto:
		// Proceed without asking.
	default:
		return Outcome{}, fmt.Errorf("executor: entry %q has unknown permission %q", entry.Name, entry.Permission)
	}

	route := e.Route
	if route == nil {
		route = Route
	}
	ad, err := route(entry)
	if err != nil {
		return Outcome{}, fmt.Errorf("executor: routing %q failed: %w", entry.Name, err)
	}

	result, err := ad.Invoke(ctx, entry, task)
	if err != nil {
		return Outcome{}, fmt.Errorf("executor: invoking %q failed: %w", entry.Name, err)
	}
	return Outcome{Decision: DecisionInvoked, Entry: entry, Result: result}, nil
}
