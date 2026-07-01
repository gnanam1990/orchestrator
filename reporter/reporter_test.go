package reporter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gnanam1990/orchestrator/adapter"
	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/executor"
)

func TestReport_Success(t *testing.T) {
	entry := catalog.Manifest{Name: "weather", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/w", Permission: catalog.PermissionAuto}
	out := executor.Outcome{
		Decision: executor.DecisionInvoked,
		Entry:    entry,
		Result:   adapter.Result{Output: `{"temperature":24.1}`, StatusCode: 200},
	}

	r := Build("weather in Berlin", out, nil)
	if r.Kind != KindSuccess {
		t.Fatalf("Kind = %q, want success", r.Kind)
	}
	if r.Entry != "weather" {
		t.Errorf("Entry = %q, want weather", r.Entry)
	}

	s := Format(r)
	// entry name and reason must always be present, output must be attributed.
	if !strings.Contains(s, "weather") {
		t.Errorf("success output must name the entry; got:\n%s", s)
	}
	if !strings.Contains(s, "success") {
		t.Errorf("success output must state the outcome; got:\n%s", s)
	}
	if !strings.Contains(s, "auto") {
		t.Errorf("success output must state the permission that applied; got:\n%s", s)
	}
	if !strings.Contains(s, `{"temperature":24.1}`) {
		t.Errorf("success output must include the adapter output; got:\n%s", s)
	}
}

func TestReport_NoMatch(t *testing.T) {
	r := Build("translate this poem", executor.Outcome{Decision: executor.DecisionNoMatch}, nil)
	if r.Kind != KindNoMatch {
		t.Fatalf("Kind = %q, want no-match", r.Kind)
	}
	if r.Entry != "none" {
		t.Errorf("Entry = %q, want none (attribution must not be blank)", r.Entry)
	}

	s := Format(r)
	if !strings.Contains(s, "no match") {
		t.Errorf("no-match output must say so plainly; got:\n%s", s)
	}
	if !strings.Contains(s, "none") {
		t.Errorf("no-match output must attribute to \"none\"; got:\n%s", s)
	}
}

func TestReport_RejectedNever(t *testing.T) {
	entry := catalog.Manifest{Name: "danger-tool", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/d", Permission: catalog.PermissionNever}
	r := Build("delete production", executor.Outcome{Decision: executor.DecisionRejectedNever, Entry: entry}, nil)
	if r.Kind != KindRejected {
		t.Fatalf("Kind = %q, want rejected", r.Kind)
	}

	s := Format(r)
	if !strings.Contains(s, "danger-tool") {
		t.Errorf("rejected output must name the entry; got:\n%s", s)
	}
	if !strings.Contains(s, "rejected") {
		t.Errorf("rejected output must state the outcome; got:\n%s", s)
	}
	if !strings.Contains(s, "never") {
		t.Errorf("rejected output must state the reason (never); got:\n%s", s)
	}
}

func TestReport_RejectedDeclined(t *testing.T) {
	entry := catalog.Manifest{Name: "deploy-tool", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/dep", Permission: catalog.PermissionAsk}
	r := Build("deploy now", executor.Outcome{Decision: executor.DecisionRejectedByApprover, Entry: entry}, nil)
	if r.Kind != KindRejected {
		t.Fatalf("Kind = %q, want rejected", r.Kind)
	}

	s := Format(r)
	if !strings.Contains(s, "deploy-tool") {
		t.Errorf("rejected output must name the entry; got:\n%s", s)
	}
	if !strings.Contains(s, "rejected") {
		t.Errorf("rejected output must state the outcome; got:\n%s", s)
	}
	if !strings.Contains(s, "declined") {
		t.Errorf("rejected output must state the reason (declined); got:\n%s", s)
	}
}

func TestReport_Error(t *testing.T) {
	err := errors.New(`executor: invoking "weather" failed: adapter: GET "https://x" returned non-2xx status 500`)
	r := Build("weather in Berlin", executor.Outcome{}, err)
	if r.Kind != KindError {
		t.Fatalf("Kind = %q, want error", r.Kind)
	}

	s := Format(r)
	if !strings.Contains(s, "error") {
		t.Errorf("error output must state the outcome; got:\n%s", s)
	}
	if !strings.Contains(s, "500") {
		t.Errorf("error output must state what failed and where; got:\n%s", s)
	}
}

// TestReport_ErrorAttributedToEntry: when the executor carries the entry on an
// error path, the report attributes the failure to that entry, not "none".
func TestReport_ErrorAttributedToEntry(t *testing.T) {
	entry := catalog.Manifest{Name: "weather", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/w", Permission: catalog.PermissionAuto}
	err := errors.New(`executor: invoking "weather" failed: adapter: GET "https://x" returned non-2xx status 500`)

	r := Build("weather in Berlin", executor.Outcome{Entry: entry}, err)
	if r.Kind != KindError {
		t.Fatalf("Kind = %q, want error", r.Kind)
	}
	if r.Entry != "weather" {
		t.Errorf("Entry = %q, want weather (error must be attributed to the responsible entry)", r.Entry)
	}
	if !strings.Contains(Format(r), "weather") {
		t.Errorf("error output must name the responsible entry; got:\n%s", Format(r))
	}
}

func TestReport_TimestampIsSetAndFormatted(t *testing.T) {
	r := Build("anything", executor.Outcome{Decision: executor.DecisionNoMatch}, nil)
	if r.Timestamp.IsZero() {
		t.Error("Build must stamp a timestamp")
	}
	if !strings.Contains(Format(r), r.Timestamp.Format(time.RFC3339)) {
		t.Error("Format must include the timestamp")
	}
}
