package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("workflow", cmdWorkflow) }

// workflowDef is one predefined pipeline: an ordered list of CLI invocations.
// Steps run through the normal Dispatch path, so every guardrail (validation,
// disclaimers, exit codes) applies to each step exactly as if typed by hand.
type workflowDef struct {
	name        string
	description string
	steps       func(device, db string) [][]string
}

var workflows = []workflowDef{
	{
		name:        "daily-sync",
		description: "refresh the local cache for a device, then export it as CSV",
		steps: func(device, db string) [][]string {
			return [][]string{
				{"sync", device, "--db", db},
				{"export", "--db", db, "--format", "csv"},
			}
		},
	},
	{
		name:        "compliance-check",
		description: "full public dossier plus the signal-volume index",
		steps: func(device, db string) [][]string {
			return [][]string{
				{"device-report", device},
				{"score", device},
			}
		},
	},
	{
		name:        "trend-watch",
		description: "recent-vs-prior window deltas, then a cache refresh",
		steps: func(device, db string) [][]string {
			return [][]string{
				{"emerging", device},
				{"sync", device, "--db", db},
			}
		},
	},
}

// cmdWorkflow lists or runs predefined pipelines. A failing step stops the
// pipeline immediately (fail-fast) and its exit code becomes the workflow's.
// Progress lines go to stderr so a piped step output (e.g. CSV) stays clean.
func cmdWorkflow(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("workflow")
	list := fs.Bool("list", false, "list the available workflows")
	runName := fs.String("run", "", "run one workflow by name (requires a device argument)")
	db := fs.String("db", defaultDBPath(), "path to the SQLite cache (sync/export steps)")
	if err := parse(fs, stderr, args, map[string]bool{"run": true, "db": true}); err != nil {
		return 2
	}

	switch {
	case *list && *runName != "":
		fmt.Fprintln(stderr, "workflow: use --list or --run, not both")
		return 2
	case *list:
		rows := make([]map[string]any, 0, len(workflows))
		for _, w := range workflows {
			steps := w.steps("<device>", "<db>")
			names := make([]string, len(steps))
			for i, s := range steps {
				names[i] = s[0]
			}
			rows = append(rows, map[string]any{
				"workflow":    w.name,
				"steps":       joinComma(names),
				"description": w.description,
			})
		}
		meta := cliutil.Meta{EmptyMsg: "no workflows defined"}
		if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
			fmt.Fprintf(stderr, "workflow: %v\n", err)
			return 1
		}
		return 0
	case *runName == "":
		fmt.Fprintln(stderr, "workflow: --list or --run <name> is required, e.g. workflow --run daily-sync pacemaker")
		return 2
	}

	var def *workflowDef
	for i := range workflows {
		if workflows[i].name == *runName {
			def = &workflows[i]
			break
		}
	}
	if def == nil {
		fmt.Fprintf(stderr, "workflow: unknown workflow %q; try --list\n", *runName)
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(stderr, "workflow: --run %s needs a device argument, e.g. workflow --run %s pacemaker\n", *runName, *runName)
		return 2
	}
	device := fs.Arg(0)

	steps := def.steps(device, *db)
	fmt.Fprintf(stderr, "workflow %s: %d step(s) for %q\n", def.name, len(steps), device)
	for i, step := range steps {
		fmt.Fprintf(stderr, "[%d/%d] %s...\n", i+1, len(steps), step[0])
		if code := Dispatch(ctx, stdout, stderr, step); code != 0 {
			fmt.Fprintf(stderr, "[%d/%d] %s FAILED (exit %d) — stopping workflow\n", i+1, len(steps), step[0], code)
			return code
		}
		fmt.Fprintf(stderr, "[%d/%d] %s OK\n", i+1, len(steps), step[0])
	}
	fmt.Fprintf(stderr, "workflow %s complete: %d step(s), 0 errors\n", def.name, len(steps))
	return 0
}
