package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/regulatory"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("safety", cmdSafety) }

// cmdSafety is a composite view of the public safety-signal record counts for a
// device: recalls (openFDA enforcement), adverse events by severity (MAUDE), and
// regulatory coverage across agencies. It reports counts and cited examples
// only — never a safety verdict, never causation.
func cmdSafety(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("safety")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "safety: a device name is required, e.g. safety pacemaker")
		return 2
	}
	device := fs.Arg(0)

	var rows []map[string]any
	summary := []cliutil.KV{{Key: "device", Value: device}}

	// Recalls (openFDA enforcement): total + a few cited examples.
	if src, ok := getSource("openfda_device_enforcement"); ok {
		recs, page, err := src.Fetch(ctx, sources.Query{Term: device, Limit: 3})
		if err != nil {
			fmt.Fprintf(stderr, "safety: recalls: %v\n", err)
		} else {
			summary = append(summary, cliutil.KV{Key: "recalls_total", Value: page.Total})
			for _, r := range recs {
				rows = append(rows, map[string]any{
					"kind": "recall", "id": r.ID,
					"date":        str(r.Raw["recall_initiation_date"]),
					"description": clip(str(r.Raw["product_description"]), 100),
				})
			}
		}
	}

	// Adverse events (MAUDE): serious/death counts + a few cited serious events.
	if src, ok := getSource("openfda_device_event"); ok {
		if counter, ok := src.(sources.EventCounter); ok {
			if counts, err := counter.CountEventTypes(ctx, sources.Query{Term: device}); err == nil {
				summary = append(summary,
					cliutil.KV{Key: "serious_adverse_events", Value: counts["Death"] + counts["Injury"]},
					cliutil.KV{Key: "deaths", Value: counts["Death"]},
				)
			}
		}
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Severity: "serious", Limit: 3})
		if err != nil {
			fmt.Fprintf(stderr, "safety: adverse: %v\n", err)
		} else {
			for _, r := range recs {
				rows = append(rows, map[string]any{
					"kind": "adverse_event", "id": r.ID,
					"date":        str(r.Raw["date_received"]),
					"description": str(r.Raw["event_type"]),
				})
			}
		}
	}

	// Regulatory coverage across agencies (static: which agencies are wired).
	live, skeleton := 0, 0
	for _, r := range regulatory.All() {
		if r.Available() {
			live++
		} else {
			skeleton++
		}
	}
	summary = append(summary, cliutil.KV{
		Key:   "regulatory_coverage",
		Value: fmt.Sprintf("%d live, %d skeleton (awaiting keyless API)", live, skeleton),
	})

	meta := cliutil.Meta{
		Summary:  summary,
		Legend:   cliutil.FDAClassLegend,
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "safety: %v\n", err)
		return 1
	}
	return 0
}
