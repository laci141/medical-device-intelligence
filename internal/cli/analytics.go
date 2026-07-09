package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("analytics", cmdAnalytics) }

// cmdAnalytics shows server-side value distributions for a device: MAUDE
// adverse events by event_type and recalls by classification. Whole-result-set
// counts via the count APIs, never a fetched page tally.
func cmdAnalytics(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("analytics")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "analytics: a device name is required, e.g. analytics pacemaker")
		return 2
	}
	device := fs.Arg(0)

	var rows []map[string]any
	addDist := func(dimension string, counts map[string]int) {
		terms := make([]string, 0, len(counts))
		for t := range counts {
			terms = append(terms, t)
		}
		// Largest first; ties break alphabetically for deterministic output.
		sort.Slice(terms, func(i, j int) bool {
			if counts[terms[i]] != counts[terms[j]] {
				return counts[terms[i]] > counts[terms[j]]
			}
			return terms[i] < terms[j]
		})
		for _, t := range terms {
			rows = append(rows, map[string]any{
				"dimension": dimension, "term": t, "count": counts[t],
			})
		}
	}

	if src, ok := getSource("openfda_device_event"); ok {
		if counter, ok := src.(sources.EventCounter); ok {
			if counts, err := counter.CountEventTypes(ctx, sources.Query{Term: device}); err != nil {
				fmt.Fprintf(stderr, "analytics: event types: %v\n", err)
			} else {
				addDist("maude_event_type", counts)
			}
		}
	}
	if src, ok := getSource("openfda_device_enforcement"); ok {
		if counter, ok := src.(sources.FieldCounter); ok {
			if counts, err := counter.CountField(ctx, sources.Query{Term: device}, "classification.exact"); err != nil {
				fmt.Fprintf(stderr, "analytics: recall classes: %v\n", err)
			} else {
				addDist("recall_classification", counts)
			}
		}
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "device", Value: device},
			{Key: "note", Value: volumeNote},
		},
		Legend:   cliutil.FDAClassLegend,
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "analytics: %v\n", err)
		return 1
	}
	return 0
}
