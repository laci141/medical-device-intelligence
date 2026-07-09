package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("trials", cmdTrials) }

// cmdTrials lists ClinicalTrials.gov studies whose intervention type is DEVICE
// and whose intervention matches the term. Each row cites its NCT id. Trial
// registration is not evidence of efficacy or approval.
func cmdTrials(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("trials")
	limit := fs.Int("limit", 10, "max trials to list (>=1)")
	firm := fs.String("firm", "", "filter by sponsor name")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true, "firm": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "trials: a device name is required, e.g. trials pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "trials: --limit must be >= 1")
		return 2
	}
	device := fs.Arg(0)

	src, ok := getSource("clinicaltrials")
	if !ok {
		fmt.Fprintln(stderr, "trials: clinicaltrials source unavailable")
		return 1
	}
	recs, page, err := src.Fetch(ctx, sources.Query{Term: device, Firm: *firm, Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "trials: %v\n", err)
		return 1
	}

	rows := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, map[string]any{
			"nct_id":        r.ID,
			"status":        str(r.Raw["status"]),
			"phase":         str(r.Raw["phase"]),
			"title":         clip(str(r.Raw["title"]), 120),
			"conditions":    clip(str(r.Raw["conditions"]), 100),
			"interventions": clip(str(r.Raw["interventions"]), 100),
		})
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "device", Value: device},
			{Key: "trials_total", Value: page.Total},
			{Key: "note", Value: "device-intervention studies only; registration is not proof of efficacy"},
		},
		EmptyMsg: "no device trials found on ClinicalTrials.gov",
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "trials: %v\n", err)
		return 1
	}
	return 0
}
