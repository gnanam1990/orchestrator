// Command orchestrator is the CLI entry point for the agent orchestrator.
//
// It exposes three commands:
//   - catalog list [-dir DIR]      load a manifest directory and list its entries
//   - select [-dir DIR] "<task>"   pick the delegate entry best suited to a task
//   - run [-dir DIR] "<task>"      select, apply permission policy, and invoke
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/gnanam1990/orchestrator/catalog"
	"github.com/gnanam1990/orchestrator/executor"
	"github.com/gnanam1990/orchestrator/reporter"
	"github.com/gnanam1990/orchestrator/selector"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
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
	// reporter, so this is the only place run's output is produced. A catalog
	// load failure above is a setup error, not a run outcome, so it still exits
	// via the CLI error path.
	sel := selector.New(selector.NewAnthropicCaller())
	outcome, execErr := executor.New().Execute(context.Background(), task, manifests, sel, executor.NewStdinApprover())

	fmt.Print(reporter.Format(reporter.Build(task, outcome, execErr)))
	return nil
}
