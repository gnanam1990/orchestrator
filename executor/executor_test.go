package executor

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gnanam1990/orchestrator/adapter"
	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/selector"
)

// --- fakes (no real API, stdin, or network) ---

// fakeLLM drives the selector deterministically.
type fakeLLM struct {
	response string
	err      error
}

func (f fakeLLM) Complete(_ context.Context, _ string) (string, error) {
	return f.response, f.err
}

// fakeApprover records whether it was consulted and returns a scripted answer.
type fakeApprover struct {
	approve bool
	err     error
	calls   int
}

func (f *fakeApprover) Approve(_ context.Context, _ catalog.Manifest, _ string) (bool, error) {
	f.calls++
	return f.approve, f.err
}

// spyAdapter records whether Invoke was called and returns a scripted result.
type spyAdapter struct {
	calls  int
	result adapter.Result
	err    error
}

func (s *spyAdapter) Invoke(_ context.Context, _ catalog.Manifest, _ string) (adapter.Result, error) {
	s.calls++
	return s.result, s.err
}

// --- fixtures ---

func testEntries() []catalog.Manifest {
	return []catalog.Manifest{
		{Name: "auto-tool", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/auto", Description: "auto", Permission: catalog.PermissionAuto},
		{Name: "ask-tool", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/ask", Description: "ask", Permission: catalog.PermissionAsk},
		{Name: "never-tool", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/never", Description: "never", Permission: catalog.PermissionNever},
		{Name: "claude-code-cli", Type: catalog.TypeDelegate, Adapter: catalog.AdapterCompliant, Invoke: `claude --bare -p "{task}"`, Description: "cli", Permission: catalog.PermissionAsk},
	}
}

// selectorReturning builds a selector whose fake LLM always names name.
func selectorReturning(name string) *selector.Selector {
	return selector.New(fakeLLM{response: name})
}

// spyExecutor returns an Executor whose router always yields spy.
func spyExecutor(spy adapter.Adapter) *Executor {
	return &Executor{Route: func(catalog.Manifest) (adapter.Adapter, error) { return spy, nil }}
}

// --- Execute: permission engine ---

func TestExecute_AutoProceedsWithoutApproval(t *testing.T) {
	spy := &spyAdapter{result: adapter.Result{Output: "auto-output", StatusCode: 200}}
	appr := &fakeApprover{approve: true}

	out, err := spyExecutor(spy).Execute(context.Background(), "do it", testEntries(), selectorReturning("auto-tool"), appr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != DecisionInvoked {
		t.Fatalf("Decision = %v, want Invoked", out.Decision)
	}
	if out.Entry.Name != "auto-tool" {
		t.Errorf("Entry = %q, want auto-tool", out.Entry.Name)
	}
	if out.Result.Output != "auto-output" {
		t.Errorf("Result.Output = %q, want auto-output", out.Result.Output)
	}
	if appr.calls != 0 {
		t.Errorf("Approve must not be called for an auto entry; calls = %d", appr.calls)
	}
	if spy.calls != 1 {
		t.Errorf("adapter should be invoked once; calls = %d", spy.calls)
	}
}

func TestExecute_AskApprovedProceeds(t *testing.T) {
	spy := &spyAdapter{result: adapter.Result{Output: "ask-output"}}
	appr := &fakeApprover{approve: true}

	out, err := spyExecutor(spy).Execute(context.Background(), "do it", testEntries(), selectorReturning("ask-tool"), appr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != DecisionInvoked {
		t.Fatalf("Decision = %v, want Invoked", out.Decision)
	}
	if appr.calls != 1 {
		t.Errorf("Approve calls = %d, want 1", appr.calls)
	}
	if spy.calls != 1 {
		t.Errorf("adapter calls = %d, want 1", spy.calls)
	}
}

func TestExecute_AskRejectedCleanly(t *testing.T) {
	spy := &spyAdapter{}
	appr := &fakeApprover{approve: false}

	out, err := spyExecutor(spy).Execute(context.Background(), "do it", testEntries(), selectorReturning("ask-tool"), appr)
	if err != nil {
		t.Fatalf("a rejection must not be an error; got: %v", err)
	}
	if out.Decision != DecisionRejectedByApprover {
		t.Fatalf("Decision = %v, want RejectedByApprover", out.Decision)
	}
	if out.Entry.Name != "ask-tool" {
		t.Errorf("Entry = %q, want ask-tool", out.Entry.Name)
	}
	if appr.calls != 1 {
		t.Errorf("Approve calls = %d, want 1", appr.calls)
	}
	if spy.calls != 0 {
		t.Errorf("adapter must not be invoked on rejection; calls = %d", spy.calls)
	}
}

func TestExecute_NeverRejectedWithoutApproval(t *testing.T) {
	spy := &spyAdapter{}
	appr := &fakeApprover{approve: true} // would approve if it were ever asked

	out, err := spyExecutor(spy).Execute(context.Background(), "do it", testEntries(), selectorReturning("never-tool"), appr)
	if err != nil {
		t.Fatalf("a never-rejection must not be an error; got: %v", err)
	}
	if out.Decision != DecisionRejectedNever {
		t.Fatalf("Decision = %v, want RejectedNever", out.Decision)
	}
	if appr.calls != 0 {
		t.Errorf("Approve must not be called for a never entry; calls = %d", appr.calls)
	}
	if spy.calls != 0 {
		t.Errorf("adapter must not be invoked for a never entry; calls = %d", spy.calls)
	}
}

func TestExecute_NoMatchCleanly(t *testing.T) {
	spy := &spyAdapter{}
	appr := &fakeApprover{}

	out, err := spyExecutor(spy).Execute(context.Background(), "do it", testEntries(), selectorReturning("NONE"), appr)
	if err != nil {
		t.Fatalf("no-match must not be an error; got: %v", err)
	}
	if out.Decision != DecisionNoMatch {
		t.Fatalf("Decision = %v, want NoMatch", out.Decision)
	}
	if appr.calls != 0 || spy.calls != 0 {
		t.Errorf("nothing should run on no-match; approve=%d adapter=%d", appr.calls, spy.calls)
	}
}

// TestExecute_NonURLInvokeIsRoutingError uses the REAL router: a compliant entry
// whose invoke isn't URL-shaped must surface a clear routing error, not a
// misrouted HTTP call.
func TestExecute_NonURLInvokeIsRoutingError(t *testing.T) {
	appr := &fakeApprover{approve: true}

	out, err := New().Execute(context.Background(), "do it", testEntries(), selectorReturning("claude-code-cli"), appr)
	if err == nil {
		t.Fatalf("expected a routing error for a non-URL invoke; got outcome %+v", out)
	}
	if !strings.Contains(err.Error(), "claude-code-cli") || !strings.Contains(err.Error(), "adapter") {
		t.Errorf("error should name the entry and explain no adapter is implemented; got: %v", err)
	}
	// Approval (permission "ask") happened before routing failed.
	if appr.calls != 1 {
		t.Errorf("Approve calls = %d, want 1", appr.calls)
	}
}

// --- Route ---

func TestRoute_HTTPSEntryRoutesToHTTPAdapter(t *testing.T) {
	ad, err := Route(catalog.Manifest{Name: "x", Adapter: catalog.AdapterCompliant, Invoke: "https://example.test/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := ad.(*adapter.HTTPAdapter); !ok {
		t.Errorf("expected *adapter.HTTPAdapter, got %T", ad)
	}
}

func TestRoute_HTTPEntryRoutes(t *testing.T) {
	if _, err := Route(catalog.Manifest{Name: "x", Adapter: catalog.AdapterCompliant, Invoke: "http://example.test/x"}); err != nil {
		t.Errorf("http:// should route; got: %v", err)
	}
}

func TestRoute_NonURLEntryNotSupported(t *testing.T) {
	ad, err := Route(catalog.Manifest{Name: "claude-code-cli", Adapter: catalog.AdapterCompliant, Invoke: `claude --bare -p "{task}"`})
	if err == nil {
		t.Fatalf("expected a not-supported error; got adapter %v", ad)
	}
	if ad != nil {
		t.Errorf("adapter must be nil on error; got %v", ad)
	}
	if !strings.Contains(err.Error(), "claude-code-cli") {
		t.Errorf("error should name the entry; got: %v", err)
	}
}

// --- StdinApprover (injected reader, not real stdin) ---

func TestStdinApprover_Yes(t *testing.T) {
	var out bytes.Buffer
	appr := &StdinApprover{In: strings.NewReader("y\n"), Out: &out}

	ok, err := appr.Approve(context.Background(), catalog.Manifest{Name: "ask-tool", Permission: catalog.PermissionAsk}, "my task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("expected approval for 'y'")
	}
	if s := out.String(); !strings.Contains(s, "ask-tool") || !strings.Contains(s, "my task") {
		t.Errorf("prompt should include the entry name and task; got: %q", s)
	}
}

func TestStdinApprover_No(t *testing.T) {
	appr := &StdinApprover{In: strings.NewReader("n\n"), Out: io.Discard}
	ok, err := appr.Approve(context.Background(), catalog.Manifest{Name: "x", Permission: catalog.PermissionAsk}, "t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected denial for 'n'")
	}
}

func TestStdinApprover_EmptyDefaultsNo(t *testing.T) {
	appr := &StdinApprover{In: strings.NewReader("\n"), Out: io.Discard}
	ok, err := appr.Approve(context.Background(), catalog.Manifest{Name: "x", Permission: catalog.PermissionAsk}, "t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("empty input must default to no")
	}
}
