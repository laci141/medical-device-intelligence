package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("evidence", cmdEvidence) }

// cmdEvidence is a composite view of the public clinical-evidence record
// volumes for a device: registered device trials (ClinicalTrials.gov) and
// indexed literature (PubMed), each with cited examples. It reports counts
// only — record volume is never proof of efficacy or safety.
func cmdEvidence(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("evidence")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "evidence: a device name is required, e.g. evidence pacemaker")
		return 2
	}
	device := fs.Arg(0)

	var rows []map[string]any
	summary := []cliutil.KV{{Key: "device", Value: device}}

	// Trials (ClinicalTrials.gov): total + a few cited examples.
	if src, ok := getSource("clinicaltrials"); ok {
		recs, page, err := src.Fetch(ctx, sources.Query{Term: device, Limit: 3})
		if err != nil {
			fmt.Fprintf(stderr, "evidence: trials: %v\n", err)
		} else {
			summary = append(summary, cliutil.KV{Key: "trials_total", Value: page.Total})
			for _, r := range recs {
				rows = append(rows, map[string]any{
					"kind": "trial", "id": r.ID,
					"when":        str(r.Raw["status"]),
					"description": clip(str(r.Raw["title"]), 100),
				})
			}
		}
	}

	// Publications (PubMed): total + a few cited examples.
	if src, ok := getSource("pubmed"); ok {
		recs, page, err := src.Fetch(ctx, sources.Query{Term: device, Limit: 3})
		if err != nil {
			fmt.Fprintf(stderr, "evidence: publications: %v\n", err)
		} else {
			summary = append(summary, cliutil.KV{Key: "publications_total", Value: page.Total})
			for _, r := range recs {
				rows = append(rows, map[string]any{
					"kind": "publication", "id": r.ID,
					"when":        str(r.Raw["year"]),
					"description": clip(str(r.Raw["title"]), 100),
				})
			}
		}
	}

	summary = append(summary, cliutil.KV{
		Key: "note", Value: "record volumes are public-registry counts, not proof of efficacy or safety",
	})

	meta := cliutil.Meta{
		Summary:  summary,
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "evidence: %v\n", err)
		return 1
	}
	return 0
}
