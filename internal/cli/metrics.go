package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/sources"
)

// deviceCounts is the cross-source record-volume snapshot for one device term.
// Every number is a server-side total (Page.Total or a count API), never the
// length of one fetched page, so a --limit can't skew it.
type deviceCounts struct {
	Recalls       int
	Class1Recalls int
	Serious       int
	Deaths        int
	Trials        int
	Publications  int
}

// fetchTotal returns the server-side total for a query with a minimal page.
func fetchTotal(ctx context.Context, src sources.Source, q sources.Query) (int, error) {
	q.Limit = 1
	_, page, err := src.Fetch(ctx, q)
	return page.Total, err
}

// gatherCounts collects the snapshot across the live sources. A failing source
// is reported on stderr and its numbers stay zero — partial data is labeled,
// never silently invented.
func gatherCounts(ctx context.Context, stderr io.Writer, device string) deviceCounts {
	var c deviceCounts
	if src, ok := getSource("openfda_device_enforcement"); ok {
		if n, err := fetchTotal(ctx, src, sources.Query{Term: device}); err != nil {
			fmt.Fprintf(stderr, "counts: recalls: %v\n", err)
		} else {
			c.Recalls = n
		}
		if n, err := fetchTotal(ctx, src, sources.Query{Term: device, Class: 1}); err != nil {
			fmt.Fprintf(stderr, "counts: class-1 recalls: %v\n", err)
		} else {
			c.Class1Recalls = n
		}
	}
	if src, ok := getSource("openfda_device_event"); ok {
		if counter, ok := src.(sources.EventCounter); ok {
			if counts, err := counter.CountEventTypes(ctx, sources.Query{Term: device}); err != nil {
				fmt.Fprintf(stderr, "counts: adverse events: %v\n", err)
			} else {
				c.Deaths = counts["Death"]
				c.Serious = counts["Death"] + counts["Injury"]
			}
		}
	}
	if src, ok := getSource("clinicaltrials"); ok {
		if n, err := fetchTotal(ctx, src, sources.Query{Term: device}); err != nil {
			fmt.Fprintf(stderr, "counts: trials: %v\n", err)
		} else {
			c.Trials = n
		}
	}
	if src, ok := getSource("pubmed"); ok {
		if n, err := fetchTotal(ctx, src, sources.Query{Term: device}); err != nil {
			fmt.Fprintf(stderr, "counts: publications: %v\n", err)
		} else {
			c.Publications = n
		}
	}
	return c
}

// volumeNote is the shared caveat for every record-volume view.
const volumeNote = "record volumes track device ubiquity and reporting practices, not risk"
