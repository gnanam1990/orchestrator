// Package reporter turns an executor outcome into an attributed, human-readable
// report: every result names the catalog entry (or "none") responsible for it,
// so output is never shown without saying which entry produced it.
//
// This is step 5: the minimal reporter. The verifier, self-check, and
// CLI-subprocess adapter are intentionally not built here.
package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/executor"
)

// OutcomeKind is the coarse classification of a run.
type OutcomeKind string

const (
	KindSuccess  OutcomeKind = "success"
	KindNoMatch  OutcomeKind = "no-match"
	KindRejected OutcomeKind = "rejected"
	KindError    OutcomeKind = "error"
)

// Report is the attributed record of a single run. Entry is always set — the
// matched entry's name, or "none" — so the result is never unattributed.
type Report struct {
	Task       string      // the original task
	Kind       OutcomeKind // success / no-match / rejected / error
	Entry      string      // matched entry name, or "none"
	Permission string      // the permission that applied (auto/ask/never), or "" when no entry matched
	Output     string      // raw adapter output; populated only on success
	Err        string      // error detail; populated only on error
	Timestamp  time.Time   // when the report was built
}

// Build constructs a Report from the result of executor.Execute — its Outcome
// and error. A non-nil err is the error outcome; otherwise the outcome's
// Decision classifies the result. Entry is always attributed (name or "none").
func Build(task string, outcome executor.Outcome, err error) Report {
	r := Report{
		Task:      task,
		Entry:     "none",
		Timestamp: time.Now(),
	}

	if err != nil {
		r.Kind = KindError
		r.Err = err.Error()
		// Attribute to the entry if the executor got far enough to know it.
		if outcome.Entry.Name != "" {
			r.Entry = outcome.Entry.Name
			r.Permission = string(outcome.Entry.Permission)
		}
		return r
	}

	switch outcome.Decision {
	case executor.DecisionInvoked:
		r.Kind = KindSuccess
		r.Entry = entryName(outcome.Entry)
		r.Permission = string(outcome.Entry.Permission)
		r.Output = outcome.Result.Output
	case executor.DecisionNoMatch:
		r.Kind = KindNoMatch // Entry stays "none".
	case executor.DecisionRejectedNever, executor.DecisionRejectedByApprover:
		r.Kind = KindRejected
		r.Entry = entryName(outcome.Entry)
		r.Permission = string(outcome.Entry.Permission)
	default:
		r.Kind = KindError
		r.Err = fmt.Sprintf("reporter: unexpected executor decision %v", outcome.Decision)
	}
	return r
}

// Format renders a Report as clean CLI text with distinct phrasing per outcome
// kind. The Entry line is always present, so no result is shown unattributed.
func Format(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:    %s\n", r.Task)
	fmt.Fprintf(&b, "Entry:   %s\n", r.Entry)

	switch r.Kind {
	case KindSuccess:
		fmt.Fprintf(&b, "Outcome: success — %q ran (permission: %s)\n", r.Entry, r.Permission)
		b.WriteString("Output:\n")
		b.WriteString(ensureTrailingNewline(r.Output))
	case KindNoMatch:
		b.WriteString("Outcome: no match — no catalog entry fit this task\n")
	case KindRejected:
		// Build only produces KindRejected for a "never" or "ask" entry, so the
		// permission cleanly distinguishes policy-forbidden from approval-declined.
		if r.Permission == string(catalog.PermissionNever) {
			fmt.Fprintf(&b, "Outcome: rejected — %q may not run (permission: never)\n", r.Entry)
		} else {
			fmt.Fprintf(&b, "Outcome: rejected — approval for %q was declined\n", r.Entry)
		}
	case KindError:
		fmt.Fprintf(&b, "Outcome: error — %s\n", r.Err)
	}

	fmt.Fprintf(&b, "When:    %s\n", r.Timestamp.Format(time.RFC3339))
	return b.String()
}

// entryName returns the entry's name, or "none" when unset, so attribution is
// never blank.
func entryName(entry catalog.Manifest) string {
	if entry.Name == "" {
		return "none"
	}
	return entry.Name
}

// ensureTrailingNewline keeps the rendered output block tidy.
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
