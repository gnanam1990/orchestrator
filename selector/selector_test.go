package selector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gnanam1990/orchestrator/catalog"
)

// fakeCaller is a scripted LLMCaller: it records the prompt it was given and
// returns a canned response/error, so tests never touch a real API.
type fakeCaller struct {
	response   string
	err        error
	calls      int
	lastPrompt string
}

func (f *fakeCaller) Complete(_ context.Context, prompt string) (string, error) {
	f.calls++
	f.lastPrompt = prompt
	return f.response, f.err
}

// delegateEntries returns two delegate manifests plus one knowledge entry (to
// exercise filtering).
func delegateEntries() []catalog.Manifest {
	return []catalog.Manifest{
		{
			Name:        "claude-code-cli",
			Type:        catalog.TypeDelegate,
			Adapter:     catalog.AdapterCompliant,
			Invoke:      `claude --bare -p "{task}"`,
			Description: "Delegate a coding task to the Claude Code CLI.",
			Permission:  catalog.PermissionAsk,
		},
		{
			Name:        "weather",
			Type:        catalog.TypeDelegate,
			Adapter:     catalog.AdapterCompliant,
			Invoke:      "https://api.open-meteo.com/v1/forecast",
			Description: "Fetch a weather forecast for a location.",
			Permission:  catalog.PermissionAuto,
		},
		{
			Name:        "house-style",
			Type:        catalog.TypeKnowledge,
			Description: "The team coding conventions.",
		},
	}
}

// TestSelect_PicksWeather routes a weather task to the weather entry.
func TestSelect_PicksWeather(t *testing.T) {
	fake := &fakeCaller{response: "weather"}
	got, err := New(fake).Select(context.Background(), "what's the forecast in Berlin tomorrow?", delegateEntries())
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if !got.Matched || got.Entry.Name != "weather" {
		t.Fatalf("expected a match on weather; got %+v", got)
	}
	// The prompt should list candidates by name so the model can choose.
	if !strings.Contains(fake.lastPrompt, "weather") || !strings.Contains(fake.lastPrompt, "claude-code-cli") {
		t.Errorf("prompt should list candidate names; got:\n%s", fake.lastPrompt)
	}
	// Knowledge entries are filtered out and must not appear as candidates.
	if strings.Contains(fake.lastPrompt, "house-style") {
		t.Errorf("knowledge entry should not be offered as a candidate; got:\n%s", fake.lastPrompt)
	}
}

// TestSelect_PicksCodeCLI routes a coding task to claude-code-cli, and tolerates
// wrapping whitespace/quotes in the model's reply.
func TestSelect_PicksCodeCLI(t *testing.T) {
	fake := &fakeCaller{response: "  \"claude-code-cli\"\n"}
	got, err := New(fake).Select(context.Background(), "refactor this Go package and add tests", delegateEntries())
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if !got.Matched || got.Entry.Name != "claude-code-cli" {
		t.Fatalf("expected a match on claude-code-cli; got %+v", got)
	}
}

// TestSelect_NoneReturnsNoMatch treats "NONE" as a clean no-match, not an error.
func TestSelect_NoneReturnsNoMatch(t *testing.T) {
	fake := &fakeCaller{response: "NONE"}
	got, err := New(fake).Select(context.Background(), "translate this poem into French", delegateEntries())
	if err != nil {
		t.Fatalf("NONE should not be an error; got: %v", err)
	}
	if got.Matched {
		t.Fatalf("expected no match for NONE; got %+v", got)
	}
}

// TestSelect_HallucinatedNameIsError rejects a name that is not a real
// candidate rather than silently accepting it.
func TestSelect_HallucinatedNameIsError(t *testing.T) {
	fake := &fakeCaller{response: "database-writer"}
	got, err := New(fake).Select(context.Background(), "store this record", delegateEntries())
	if err == nil {
		t.Fatalf("expected an error for a non-candidate name; got result %+v", got)
	}
	if !strings.Contains(err.Error(), "database-writer") {
		t.Errorf("error should name the offending answer; got: %v", err)
	}
	if got.Matched {
		t.Errorf("result must not report a match when the answer is invalid; got %+v", got)
	}
}

// TestSelect_LLMErrorPropagates surfaces a caller error.
func TestSelect_LLMErrorPropagates(t *testing.T) {
	sentinel := errors.New("network down")
	fake := &fakeCaller{err: sentinel}
	_, err := New(fake).Select(context.Background(), "anything", delegateEntries())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the caller error to propagate; got: %v", err)
	}
}

// TestSelect_SentinelNameCollisionErrors refuses (loudly) a delegate whose name
// collides with the no-match sentinel, rather than silently shadowing it.
func TestSelect_SentinelNameCollisionErrors(t *testing.T) {
	fake := &fakeCaller{response: "NONE"}
	entries := []catalog.Manifest{
		{
			Name:        "none", // case-insensitively equals the "NONE" sentinel
			Type:        catalog.TypeDelegate,
			Adapter:     catalog.AdapterCompliant,
			Invoke:      "run-me",
			Description: "A delegate that collides with the sentinel.",
			Permission:  catalog.PermissionAuto,
		},
	}
	got, err := New(fake).Select(context.Background(), "anything", entries)
	if err == nil {
		t.Fatalf("expected an error for a sentinel-colliding delegate name; got result %+v", got)
	}
	if !strings.Contains(err.Error(), "sentinel") {
		t.Errorf("error should explain the sentinel collision; got: %v", err)
	}
	if got.Matched {
		t.Errorf("result must not report a match on the error path; got %+v", got)
	}
}

// TestSelect_NoDelegatesShortCircuits returns no-match without calling the LLM
// when there are no delegate candidates.
func TestSelect_NoDelegatesShortCircuits(t *testing.T) {
	fake := &fakeCaller{response: "weather"} // would wrongly match if consulted
	knowledgeOnly := []catalog.Manifest{
		{Name: "house-style", Type: catalog.TypeKnowledge, Description: "conventions"},
	}
	got, err := New(fake).Select(context.Background(), "anything", knowledgeOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Matched {
		t.Fatalf("expected no match with no delegates; got %+v", got)
	}
	if fake.calls != 0 {
		t.Errorf("LLM should not be called when there are no candidates; calls=%d", fake.calls)
	}
}
