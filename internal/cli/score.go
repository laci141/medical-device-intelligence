package cli

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("score", cmdScore) }

// Score weights. The index is a TRANSPARENT log-scaled sum of public
// record volumes — the formula is printed with every result. It is
// deliberately NOT called a risk or safety score: record volume tracks how
// widely a device is used and reported, not how dangerous it is.
var scoreWeights = []struct {
	component string
	weight    float64
	count     func(deviceCounts) int
}{
	{"recalls_total", 10, func(c deviceCounts) int { return c.Recalls }},
	{"class1_recalls", 15, func(c deviceCounts) int { return c.Class1Recalls }},
	{"serious_adverse_events", 10, func(c deviceCounts) int { return c.Serious }},
	{"deaths", 15, func(c deviceCounts) int { return c.Deaths }},
}

const scoreFormula = "sum over components of weight * log10(1 + count)"

// cmdScore computes the signal-volume index for a device from server-side
// totals and shows every component, weight, and point contribution.
func cmdScore(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("score")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "score: a device name is required, e.g. score pacemaker")
		return 2
	}
	device := fs.Arg(0)

	c := gatherCounts(ctx, stderr, device)

	var rows []map[string]any
	total := 0.0
	for _, w := range scoreWeights {
		n := w.count(c)
		points := w.weight * math.Log10(1+float64(n))
		total += points
		rows = append(rows, map[string]any{
			"component": w.component,
			"count":     n,
			"weight":    w.weight,
			"points":    fmt.Sprintf("%.1f", points),
		})
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "device", Value: device},
			{Key: "signal_volume_index", Value: fmt.Sprintf("%.1f", total)},
			{Key: "formula", Value: scoreFormula},
			{Key: "note", Value: "NOT a risk or safety score; " + volumeNote},
		},
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "score: %v\n", err)
		return 1
	}
	return 0
}
