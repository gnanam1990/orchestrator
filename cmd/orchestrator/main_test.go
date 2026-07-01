package main

import (
	"testing"

	"github.com/gnanam1990/orchestrator/reporter"
)

// TestRunExitCode asserts the process exit code per run outcome kind: only an
// "error" outcome is a failure; success, no-match, and rejected are all
// legitimate expected outcomes and exit 0.
func TestRunExitCode(t *testing.T) {
	cases := []struct {
		kind reporter.OutcomeKind
		want int
	}{
		{reporter.KindSuccess, 0},
		{reporter.KindNoMatch, 0},
		{reporter.KindRejected, 0},
		{reporter.KindError, 1},
	}
	for _, c := range cases {
		if got := runExitCode(c.kind); got != c.want {
			t.Errorf("runExitCode(%q) = %d, want %d", c.kind, got, c.want)
		}
	}
}
