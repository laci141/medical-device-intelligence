package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/regulatory"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("device-report", cmdDeviceReport) }

// cmdDeviceReport assembles the full public dossier for one device: identity
// (GUDID), recalls, serious adverse events, device trials, and literature —
// server-side totals in the headline, cited examples as rows. Facts and counts
// only; never a safety verdict.
func cmdDeviceReport(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("device-report")
	examples := fs.Int("examples", 3, "cited examples per section (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"examples": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "device-report: a device name is required, e.g. device-report pacemaker")
		return 2
	}
	if *examples < 1 {
		fmt.Fprintln(stderr, "device-report: --examples must be >= 1")
		return 2
	}
	device := fs.Arg(0)

	c := gatherCounts(ctx, stderr, device)
	summary := []cliutil.KV{
		{Key: "device", Value: device},
		{Key: "recalls_total", Value: c.Recalls},
		{Key: "class1_recalls", Value: c.Class1Recalls},
		{Key: "serious_adverse_events", Value: c.Serious},
		{Key: "deaths", Value: c.Deaths},
		{Key: "trials_total", Value: c.Trials},
		{Key: "publications_total", Value: c.Publications},
	}
	live, skeleton := 0, 0
	for _, r := range regulatory.All() {
		if r.Available() {
			live++
		} else {
			skeleton++
		}
	}
	summary = append(summary,
		cliutil.KV{Key: "regulatory_coverage", Value: fmt.Sprintf("%d live, %d skeleton (awaiting keyless API)", live, skeleton)},
		cliutil.KV{Key: "note", Value: volumeNote},
	)

	var rows []map[string]any
	addRows := func(kind string, recs []sources.RawRecord, when, desc func(sources.RawRecord) string) {
		for _, r := range recs {
			rows = append(rows, map[string]any{
				"kind": kind, "id": r.ID, "when": when(r), "description": clip(desc(r), 100),
			})
		}
	}

	if src, ok := getSource("openfda_device_udi"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: 1})
		if err != nil {
			fmt.Fprintf(stderr, "device-report: udi: %v\n", err)
		} else {
			addRows("identity", recs,
				func(r sources.RawRecord) string { return str(r.Raw["publish_date"]) },
				func(r sources.RawRecord) string {
					return str(r.Raw["brand_name"]) + " — " + str(r.Raw["company_name"])
				})
		}
	}
	if src, ok := getSource("openfda_device_enforcement"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: *examples})
		if err != nil {
			fmt.Fprintf(stderr, "device-report: recalls: %v\n", err)
		} else {
			addRows("recall", recs,
				func(r sources.RawRecord) string { return str(r.Raw["recall_initiation_date"]) },
				func(r sources.RawRecord) string { return str(r.Raw["product_description"]) })
		}
	}
	if src, ok := getSource("openfda_device_event"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Severity: "serious", Limit: *examples})
		if err != nil {
			fmt.Fprintf(stderr, "device-report: adverse: %v\n", err)
		} else {
			addRows("serious_event", recs,
				func(r sources.RawRecord) string { return str(r.Raw["date_received"]) },
				func(r sources.RawRecord) string { return str(r.Raw["event_type"]) })
		}
	}
	if src, ok := getSource("clinicaltrials"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: *examples})
		if err != nil {
			fmt.Fprintf(stderr, "device-report: trials: %v\n", err)
		} else {
			addRows("trial", recs,
				func(r sources.RawRecord) string { return str(r.Raw["status"]) },
				func(r sources.RawRecord) string { return str(r.Raw["title"]) })
		}
	}
	if src, ok := getSource("pubmed"); ok {
		recs, _, err := src.Fetch(ctx, sources.Query{Term: device, Limit: *examples})
		if err != nil {
			fmt.Fprintf(stderr, "device-report: publications: %v\n", err)
		} else {
			addRows("publication", recs,
				func(r sources.RawRecord) string { return str(r.Raw["year"]) },
				func(r sources.RawRecord) string { return str(r.Raw["title"]) })
		}
	}

	meta := cliutil.Meta{
		Summary:  summary,
		Legend:   cliutil.FDAClassLegend,
		EmptyMsg: cliutil.NoRecordsMsg,
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "device-report: %v\n", err)
		return 1
	}
	return 0
}
