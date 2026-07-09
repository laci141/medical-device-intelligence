package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("summary", cmdSummary) }

// cmdSummary gives a brief recall overview for a device: total recalls seen, a
// per-classification breakdown, and the cited recall_number of each row folded
// into the counts. It is a correlational summary of public records, never a
// safety judgment.
func cmdSummary(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("summary")
	limit := fs.Int("limit", 100, "max records to summarize (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "summary: a device name is required, e.g. summary pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "summary: --limit must be >= 1")
		return 2
	}
	term := fs.Arg(0)

	src, ok := getSource("openfda_device_enforcement")
	if !ok {
		fmt.Fprintln(stderr, "summary: enforcement source unavailable")
		return 1
	}
	recs, page, err := src.Fetch(ctx, sources.Query{Term: term, Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "summary: %v\n", err)
		return 1
	}

	// Count by classification and collect a few cited example ids per class.
	counts := map[string]int{}
	examples := map[string][]string{}
	for _, r := range recs {
		c := str(r.Raw["classification"])
		if c == "" {
			c = "Unclassified"
		}
		counts[c]++
		if len(examples[c]) < 3 {
			examples[c] = append(examples[c], r.ID)
		}
	}

	classes := make([]string, 0, len(counts))
	for c := range counts {
		classes = append(classes, c)
	}
	sort.Strings(classes)

	rows := make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		rows = append(rows, map[string]any{
			"classification": c,
			"count":          counts[c],
			"example_ids":    examples[c],
		})
	}

	// Headline goes in Meta.Summary so the per-class rows stay homogeneous.
	meta := cliutil.Meta{
		Legend:   cliutil.FDAClassLegend,
		EmptyMsg: cliutil.NoRecordsMsg,
		Summary: []cliutil.KV{
			{Key: "device", Value: term},
			{Key: "recalls_examined", Value: len(recs)},
			{Key: "total_available", Value: page.Total},
			{Key: "note", Value: "correlational record counts, not a safety verdict"},
		},
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "summary: %v\n", err)
		return 1
	}
	return 0
}
