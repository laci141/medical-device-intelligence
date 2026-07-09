package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("compare", cmdCompare) }

// cmdCompare puts the record-volume snapshots of two devices side by side.
// Volumes are server-side totals per source. A higher number is more public
// records, not more risk — the two devices may differ wildly in market size
// and reporting practices.
func cmdCompare(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("compare")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(stderr, "compare: two device names are required, e.g. compare pacemaker stent")
		return 2
	}
	a, b := fs.Arg(0), fs.Arg(1)
	if a == b {
		fmt.Fprintln(stderr, "compare: the two device names must differ")
		return 2
	}

	ca := gatherCounts(ctx, stderr, a)
	cb := gatherCounts(ctx, stderr, b)

	metric := func(name string, va, vb int) map[string]any {
		return map[string]any{"metric": name, a: va, b: vb}
	}
	rows := []map[string]any{
		metric("recalls_total", ca.Recalls, cb.Recalls),
		metric("class1_recalls", ca.Class1Recalls, cb.Class1Recalls),
		metric("serious_adverse_events", ca.Serious, cb.Serious),
		metric("deaths", ca.Deaths, cb.Deaths),
		metric("trials_total", ca.Trials, cb.Trials),
		metric("publications_total", ca.Publications, cb.Publications),
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "devices", Value: a + " vs " + b},
			{Key: "note", Value: volumeNote},
		},
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "compare: %v\n", err)
		return 1
	}
	return 0
}
