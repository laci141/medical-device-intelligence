package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("emerging", cmdEmerging) }

// nowFunc is an indirection so tests can pin the clock.
var nowFunc = time.Now

// cmdEmerging compares a device's record volume in the most recent N-month
// window against the N months before it, for recalls (recall_initiation_date)
// and serious adverse events (date_received). Totals are server-side
// date-range queries. MAUDE and enforcement both have reporting lag, so the
// recent window systematically undercounts — a decline is not evidence of
// improvement.
func cmdEmerging(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("emerging")
	months := fs.Int("months", 12, "window size in months (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"months": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "emerging: a device name is required, e.g. emerging pacemaker")
		return 2
	}
	if *months < 1 {
		fmt.Fprintln(stderr, "emerging: --months must be >= 1")
		return 2
	}
	device := fs.Arg(0)

	now := nowFunc()
	mid := now.AddDate(0, -*months, 0)
	old := now.AddDate(0, -2*(*months), 0)
	const day = "20060102"
	recentFrom, recentTo := mid.Format(day), now.Format(day)
	priorFrom, priorTo := old.Format(day), mid.AddDate(0, 0, -1).Format(day)

	window := func(src sources.Source, q sources.Query) (recent, prior int, err error) {
		q.DateFrom, q.DateTo = recentFrom, recentTo
		if recent, err = fetchTotal(ctx, src, q); err != nil {
			return 0, 0, err
		}
		q.DateFrom, q.DateTo = priorFrom, priorTo
		prior, err = fetchTotal(ctx, src, q)
		return recent, prior, err
	}

	var rows []map[string]any
	addMetric := func(name string, recent, prior int) {
		rows = append(rows, map[string]any{
			"metric":        name,
			"recent_window": recent,
			"prior_window":  prior,
			"change":        recent - prior,
		})
	}

	if src, ok := getSource("openfda_device_enforcement"); ok {
		if recent, prior, err := window(src, sources.Query{Term: device, DateField: "recall_initiation_date"}); err != nil {
			fmt.Fprintf(stderr, "emerging: recalls: %v\n", err)
		} else {
			addMetric("recalls", recent, prior)
		}
	}
	if src, ok := getSource("openfda_device_event"); ok {
		if recent, prior, err := window(src, sources.Query{Term: device, Severity: "serious", DateField: "date_received"}); err != nil {
			fmt.Fprintf(stderr, "emerging: serious events: %v\n", err)
		} else {
			addMetric("serious_adverse_events", recent, prior)
		}
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "device", Value: device},
			{Key: "recent_window", Value: recentFrom + " to " + recentTo},
			{Key: "prior_window", Value: priorFrom + " to " + priorTo},
			{Key: "note", Value: "reporting lag undercounts the recent window; a decline is not evidence of improvement"},
		},
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "emerging: %v\n", err)
		return 1
	}
	return 0
}
