package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("search", cmdSearch) }

// cmdSearch queries every registered source for a term and returns up to --limit
// aggregated rows, each tagged with its source and citing that source's record
// id. Staged sources (ErrNotWired) are reported as pending on stderr rather than
// failing the run.
func cmdSearch(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("search")
	limit := fs.Int("limit", 10, "max aggregated results (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "search: a term is required, e.g. search pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "search: --limit must be >= 1")
		return 2
	}
	term := fs.Arg(0)

	var rows []map[string]any
	var pending []string
	names := allSources()
	sort.Strings(names)
	for _, name := range names {
		src, ok := getSource(name)
		if !ok {
			continue
		}
		recs, _, err := src.Fetch(ctx, sources.Query{Term: term, Limit: *limit})
		if err != nil {
			if errors.Is(err, sources.ErrNotWired) {
				pending = append(pending, name)
				continue
			}
			fmt.Fprintf(stderr, "search: %s: %v\n", name, err)
			continue
		}
		for _, r := range recs {
			rows = append(rows, map[string]any{
				"source":  name,
				"id":      r.ID,
				"summary": summarize(r.Raw),
			})
		}
	}

	// Trim the aggregate to --limit.
	if len(rows) > *limit {
		rows = rows[:*limit]
	}

	if len(pending) > 0 {
		fmt.Fprintf(stderr, "note: %d source(s) staged (not yet integrated): %v\n", len(pending), pending)
	}

	meta := cliutil.Meta{EmptyMsg: cliutil.NoRecordsMsg}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "search: %v\n", err)
		return 1
	}
	return 0
}

// summarize picks the most descriptive available field for a generic result row.
func summarize(raw map[string]any) string {
	for _, k := range []string{"product_description", "brand_name", "title", "device_name", "reason_for_recall"} {
		if v := str(raw[k]); v != "" {
			return clip(v, 140)
		}
	}
	return ""
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
