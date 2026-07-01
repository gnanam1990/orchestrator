// Command orchestrator is the CLI entry point for the agent orchestrator.
//
// It exposes three commands:
//   - catalog list [-dir DIR]      load a manifest directory and list its entries
//   - select [-dir DIR] "<task>"   pick the delegate entry best suited to a task
//   - run [-dir DIR] "<task>"      select, apply permission policy, and invoke
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/executor"
	"github.com/gnanam1990/orchestrator/reporter"
	"github.com/gnanam1990/orchestrator/selector"
)

// errReported signals that a command has already presented its outcome through
// the reporter and the process should exit non-zero without printing anything
// further — this keeps the report the sole output surface.
var errReported = errors.New("reported")

func main() {
	err := run(os.Args[1:])
	if err == nil {
		return
	}
	// A reported failure has already printed its report; anything else is a
	// setup/usage error that prints to stderr. Both exit non-zero.
	if !errors.Is(err, errReported) {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: orchestrator <command>\n\ncommands:\n  catalog list [-dir DIR]        load a manifest directory and list its entries\n  select [-dir DIR] \"<task>\"      pick the delegate entry best suited to a task\n  run [-dir DIR] \"<task>\"         select, apply permission policy, and invoke")
	}
	switch args[0] {
	case "catalog":
		return runCatalog(args[1:])
	case "select":
		return runSelect(args[1:])
	case "run":
		return runRun(args[1:])
	default:
		return fmt.Errorf("unknown command %q (try: catalog list, select, run)", args[0])
	}
}

func runCatalog(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: orchestrator catalog list [-dir DIR]")
	}
	switch args[0] {
	case "list":
		return runCatalogList(args[1:])
	default:
		return fmt.Errorf("unknown catalog subcommand %q (try: list)", args[0])
	}
}

func runCatalogList(args []string) error {
	fs := flag.NewFlagSet("catalog list", flag.ContinueOnError)
	dir := fs.String("dir", "manifests", "directory containing manifest .yaml/.yml files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	manifests, err := catalog.LoadDir(*dir)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "NAME\tTYPE\tPERMISSION")
	for _, m := range manifests {
		perm := string(m.Permission)
		if perm == "" {
			perm = "-" // knowledge entries carry no permission
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.Name, m.Type, perm)
	}
	return nil
}

func runSelect(args []string) error {
	fs := flag.NewFlagSet("select", flag.ContinueOnError)
	dir := fs.String("dir", "manifests", "directory containing manifest .yaml/.yml files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: orchestrator select [-dir DIR] \"<task>\"")
	}
	task := rest[0]

	manifests, err := catalog.LoadDir(*dir)
	if err != nil {
		return err
	}

	sel := selector.New(selector.NewAnthropicCaller())
	result, err := sel.Select(context.Background(), task, manifests)
	if err != nil {
		return err
	}
	if !result.Matched {
		fmt.Println("no match")
		return nil
	}
	fmt.Println(result.Entry.Name)
	return nil
}

func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dir := fs.String("dir", "manifests", "directory containing manifest .yaml/.yml files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: orchestrator run [-dir DIR] \"<task>\"")
	}
	task := rest[0]

	manifests, err := catalog.LoadDir(*dir)
	if err != nil {
		return err
	}

	// Selection, permission, and invocation all funnel into a single report:
	// every run outcome (including a pipeline error) is presented through the
	// reporter, so this is the only place run's output is produced. The process
	// then exits 0 for the legitimate outcomes (success / no-match / rejected)
	// and 1 only when the outcome kind is "error". A catalog load failure above
	// is a setup error, not a run outcome, so it still exits via the CLI error
	// path.
	sel := selector.New(selector.NewAnthropicCaller())
	outcome, execErr := executor.New().Execute(context.Background(), task, manifests, sel, executor.NewStdinApprover())

	report := reporter.Build(task, outcome, execErr)
	fmt.Print(reporter.Format(report))
	if runExitCode(report.Kind) != 0 {
		return errReported
	}
	return nil
}

// runExitCode maps a run outcome to a process exit code. An "error" outcome is a
// real failure (1); success, no-match, and rejected are all legitimate, expected
// outcomes and exit 0. This is the single source of run's exit status.
func runExitCode(kind reporter.OutcomeKind) int {
	if kind == reporter.KindError {
		return 1
	}
	return 0
}
