package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("dossier", cmdDossier) }

// cmdDossier renders the assembled intelligence dossier for a device: the
// attention index (with its formula), the top-3 highlight signals, and the
// data-quality readings kept deliberately separate from the index. The
// attention index measures public-record activity, NOT risk.
func cmdDossier(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("dossier")
	device := fs.String("device", "", "device name to analyze (required)")
	if err := parse(fs, stderr, args, map[string]bool{"device": true}); err != nil {
		return 2
	}
	term := *device
	if term == "" && fs.NArg() > 0 {
		term = fs.Arg(0)
	}
	if term == "" {
		fmt.Fprintln(stderr, "dossier: a device is required, e.g. dossier --device pacemaker")
		return 2
	}

	d, err := synthesize(ctx, term)
	if err != nil {
		fmt.Fprintf(stderr, "dossier: %v\n", err)
		return 1
	}

	// --json/--agent emit the full dossier struct; plain/csv render the rows.
	if f.JSON || f.Agent {
		if err := cliutil.OutputValue(stdout, d); err != nil {
			fmt.Fprintf(stderr, "dossier: %v\n", err)
			return 1
		}
		return 0
	}

	rows := make([]map[string]any, 0, len(d.Highlights))
	for i, h := range d.Highlights {
		rows = append(rows, map[string]any{"rank": i + 1, "highlight": h})
	}

	summary := []cliutil.KV{
		{Key: "device", Value: d.Device},
		{Key: "attention_index", Value: d.AttentionIndex},
		{Key: "formula", Value: d.IndexFormula},
		{Key: "signals_measured", Value: d.SignalsMeasured},
	}
	for _, dq := range d.DataQuality {
		summary = append(summary, cliutil.KV{Key: "data_quality", Value: clip(dq, 100)})
	}
	if len(d.Notes) > 0 {
		summary = append(summary, cliutil.KV{Key: "partial", Value: fmt.Sprintf("%d probe(s) unavailable", len(d.Notes))})
	}
	summary = append(summary, cliutil.KV{Key: "note", Value: "attention index measures public-record activity, NOT risk"})

	meta := cliutil.Meta{
		Summary:  summary,
		EmptyMsg: "no readable signals for this device term",
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "dossier: %v\n", err)
		return 1
	}
	return 0
}
