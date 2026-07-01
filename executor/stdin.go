package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gnanam1990/orchestrator/catalog"
)

// StdinApprover asks the user to approve an "ask"-permission entry by reading a
// y/n line from stdin. In and Out default to os.Stdin/os.Stdout; they are fields
// so callers (and tests) can substitute other streams.
type StdinApprover struct {
	In  io.Reader
	Out io.Writer
}

// Compile-time check that StdinApprover satisfies Approver.
var _ Approver = (*StdinApprover)(nil)

// NewStdinApprover returns a StdinApprover bound to os.Stdin/os.Stdout.
func NewStdinApprover() *StdinApprover {
	return &StdinApprover{In: os.Stdin, Out: os.Stdout}
}

// Approve prints the entry, task, and a short reason, then reads a y/n line.
// Anything other than y/yes — including empty input or EOF — is a "no":
// approval must be explicit.
func (s *StdinApprover) Approve(_ context.Context, entry catalog.Manifest, task string) (bool, error) {
	in := s.In
	if in == nil {
		in = os.Stdin
	}
	out := s.Out
	if out == nil {
		out = os.Stdout
	}

	fmt.Fprintf(out, "Entry:  %s\n", entry.Name)
	fmt.Fprintf(out, "Task:   %s\n", task)
	fmt.Fprintf(out, "Reason: %q requires approval (permission %q)\n", entry.Name, entry.Permission)
	fmt.Fprint(out, "Approve? [y/N]: ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("executor: reading approval: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
