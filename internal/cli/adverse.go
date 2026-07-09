package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("adverse", cmdAdverse) }

// cmdAdverse reports MAUDE adverse-event records for a device. It shows a
// server-side severity breakdown (accurate for the whole result set, not one
// page) and lists the most recent events, each citing its mdr_report_key.
// MAUDE reports are correlational: a report is not proof the device caused harm.
func cmdAdverse(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("adverse")
	limit := fs.Int("limit", 10, "max events to list (>=1)")
	severity := fs.String("severity", "", "filter: serious | death (default: all)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true, "severity": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "adverse: a device name is required, e.g. adverse pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "adverse: --limit must be >= 1")
		return 2
	}
	if *severity != "" && *severity != "serious" && *severity != "death" {
		fmt.Fprintln(stderr, "adverse: --severity must be 'serious' or 'death'")
		return 2
	}
	device := fs.Arg(0)

	src, ok := getSource("openfda_device_event")
	if !ok {
		fmt.Fprintln(stderr, "adverse: MAUDE source unavailable")
		return 1
	}

	// Server-side severity breakdown, if the source supports it.
	var summary []cliutil.KV
	if counter, ok := src.(sources.EventCounter); ok {
		if counts, err := counter.CountEventTypes(ctx, sources.Query{Term: device}); err == nil {
			death := counts["Death"]
			serious := counts["Death"] + counts["Injury"]
			other := 0
			for term, n := range counts {
				if term != "Death" && term != "Injury" {
					other += n
				}
			}
			summary = []cliutil.KV{
				{Key: "device", Value: device},
				{Key: "serious_events", Value: serious},
				{Key: "deaths", Value: death},
				{Key: "other_events", Value: other},
				{Key: "note", Value: "MAUDE reports are correlational, not proof of causation"},
			}
		} else {
			fmt.Fprintf(stderr, "adverse: severity breakdown unavailable: %v\n", err)
		}
	}

	recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Severity: *severity, Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "adverse: %v\n", err)
		return 1
	}

	rows := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, map[string]any{
			"mdr_report_key": r.ID,
			"event_type":     str(r.Raw["event_type"]),
			"date_received":  str(r.Raw["date_received"]),
			"date_of_event":  str(r.Raw["date_of_event"]),
			"problems":       joinProblems(r.Raw["product_problems"]),
		})
	}

	meta := cliutil.Meta{
		Summary:  summary,
		EmptyMsg: "no adverse events found in MAUDE",
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "adverse: %v\n", err)
		return 1
	}
	return 0
}

// joinProblems renders openFDA's product_problems array (["Under-Sensing", ...])
// into a short comma string.
func joinProblems(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(arr))
	seen := map[string]bool{}
	for _, p := range arr {
		s := str(p)
		if s != "" && !seen[s] {
			seen[s] = true
			parts = append(parts, s)
		}
	}
	return clip(joinComma(parts), 120)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
