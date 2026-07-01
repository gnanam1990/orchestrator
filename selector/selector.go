// Package selector picks a single catalog entry for a task by asking an LLM,
// then validates the LLM's answer against the real candidates so a hallucinated
// name is treated as an error rather than a silent guess.
//
// This is the deliberately dumb v0: it filters to delegate entries, asks the
// model to name exactly one (or "NONE"), and matches that name back to a real
// manifest. No scoring, ranking, or adapters — just selection.
package selector

import (
	"context"
	"fmt"
	"strings"

	"github.com/gnanam1990/orchestrator/catalog"
)

// noneSentinel is the literal the LLM must return when nothing fits.
const noneSentinel = "NONE"

// LLMCaller is the minimal surface the selector needs from a language model.
// Keeping it an interface (not a concrete client) is what lets tests inject a
// fake and run without touching a real API.
type LLMCaller interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Result is the outcome of a selection. Matched reports whether an entry was
// chosen; Entry is meaningful only when Matched is true. A no-match is a normal
// result, not an error.
type Result struct {
	Matched bool
	Entry   catalog.Manifest
}

// Selector chooses catalog entries using the injected LLMCaller.
type Selector struct {
	LLM LLMCaller
}

// New returns a Selector backed by the given caller.
func New(llm LLMCaller) *Selector {
	return &Selector{LLM: llm}
}

// Select asks the LLM to choose the single delegate entry best suited to task.
//
// It filters to delegate-type entries (knowledge entries are skipped in v0),
// prompts the model to return exactly one candidate name or "NONE", and
// validates the answer against the real candidate names. An answer that matches
// no candidate is returned as an error — the raw model output is never trusted
// blindly. "NONE" (or an empty candidate set) yields Result{Matched: false}.
func (s *Selector) Select(ctx context.Context, task string, entries []catalog.Manifest) (Result, error) {
	// Filter to delegates and index them by name for validation.
	candidates := make([]catalog.Manifest, 0, len(entries))
	byName := make(map[string]catalog.Manifest, len(entries))
	for _, e := range entries {
		if e.Type != catalog.TypeDelegate {
			continue
		}
		// A delegate named like the sentinel is ambiguous — an LLM reply of
		// "NONE" could mean "pick this entry" or "nothing fits". Refuse loudly
		// rather than silently shadowing the entry into a no-match.
		if strings.EqualFold(e.Name, noneSentinel) {
			return Result{}, fmt.Errorf("selector: delegate %q collides with the reserved no-match sentinel %q", e.Name, noneSentinel)
		}
		candidates = append(candidates, e)
		byName[e.Name] = e
	}

	// Nothing to choose from → no match, without spending an API call.
	if len(candidates) == 0 {
		return Result{Matched: false}, nil
	}

	raw, err := s.LLM.Complete(ctx, buildPrompt(task, candidates))
	if err != nil {
		return Result{}, fmt.Errorf("selector: LLM call failed: %w", err)
	}

	answer := cleanAnswer(raw)
	if answer == "" {
		return Result{}, fmt.Errorf("selector: LLM returned an empty answer")
	}
	if strings.EqualFold(answer, noneSentinel) {
		return Result{Matched: false}, nil
	}

	entry, ok := byName[answer]
	if !ok {
		// The model named something that isn't a real candidate — do not guess.
		return Result{}, fmt.Errorf("selector: LLM returned %q, which is not one of the candidates", answer)
	}
	return Result{Matched: true, Entry: entry}, nil
}

// buildPrompt renders the routing prompt: the task plus a name+description line
// per candidate, with strict output instructions.
func buildPrompt(task string, candidates []catalog.Manifest) string {
	var b strings.Builder
	b.WriteString("You are a task router. Choose the single tool best suited to the user's task.\n\n")
	b.WriteString("Task:\n")
	b.WriteString(task)
	b.WriteString("\n\nAvailable tools:\n")
	for _, c := range candidates {
		fmt.Fprintf(&b, "- %s: %s\n", c.Name, c.Description)
	}
	b.WriteString("\nRespond with ONLY the exact name of the single best-matching tool from the list above, and nothing else.\n")
	b.WriteString("If none of the tools fit the task, respond with exactly: ")
	b.WriteString(noneSentinel)
	b.WriteString("\n")
	return b.String()
}

// cleanAnswer normalizes the model's reply: trims whitespace and any wrapping
// quotes or backticks the model may have added around the name.
func cleanAnswer(raw string) string {
	answer := strings.TrimSpace(raw)
	answer = strings.Trim(answer, "`\"'")
	return strings.TrimSpace(answer)
}
