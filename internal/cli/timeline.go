package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("timeline", cmdTimeline) }

// cmdTimeline merges a device's public records into one chronology, newest
// first: recalls (openFDA enforcement) and adverse events (MAUDE). FDA
// regulatory actions are the recall entries themselves (same source), so they
// are not double-counted; non-FDA agencies are skeletons. Every row cites its
// source record id. Facts only — no causation language.
func cmdTimeline(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("timeline")
	limit := fs.Int("limit", 25, "max events (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "timeline: a device name is required, e.g. timeline pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "timeline: --limit must be >= 1")
		return 2
	}
	device := fs.Arg(0)

	var rows []map[string]any

	if src, ok := getSource("openfda_device_enforcement"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: *limit})
		if err != nil {
			fmt.Fprintf(stderr, "timeline: recalls: %v\n", err)
		}
		for _, r := range recs {
			rows = append(rows, map[string]any{
				"date":        str(r.Raw["recall_initiation_date"]),
				"event_type":  "Recall (" + str(r.Raw["classification"]) + ")",
				"source":      "openFDA enforcement",
				"source_id":   r.ID,
				"description": clip(str(r.Raw["product_description"]), 90),
			})
		}
	}

	if src, ok := getSource("openfda_device_event"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: *limit})
		if err != nil {
			fmt.Fprintf(stderr, "timeline: adverse: %v\n", err)
		}
		for _, r := range recs {
			rows = append(rows, map[string]any{
				"date":        str(r.Raw["date_received"]),
				"event_type":  "MAUDE " + str(r.Raw["event_type"]),
				"source":      "openFDA MAUDE",
				"source_id":   r.ID,
				"description": joinProblems(r.Raw["product_problems"]),
			})
		}
	}

	// Sort by date descending (YYYYMMDD strings, equal length → lexicographic).
	// Empty dates sort last.
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := str(rows[i]["date"]), str(rows[j]["date"])
		if di == "" {
			return false
		}
		if dj == "" {
			return true
		}
		return di > dj
	})
	if len(rows) > *limit {
		rows = rows[:*limit]
	}

	meta := cliutil.Meta{Legend: cliutil.FDAClassLegend, EmptyMsg: cliutil.NoRecordsMsg}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "timeline: %v\n", err)
		return 1
	}
	return 0
}
