package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("recalls", cmdRecalls) }

// cmdRecalls runs a live openFDA device-recall search, optionally filtered by
// FDA class, and renders through the shared output gate. Cites recall_number.
func cmdRecalls(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("recalls")
	class := fs.Int("class", 0, "FDA class filter: 1, 2, or 3")
	limit := fs.Int("limit", 10, "max records (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"class": true, "limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "recalls: a search term is required, e.g. recalls pacemaker")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "recalls: --limit must be >= 1")
		return 2
	}
	if *class != 0 && (*class < 1 || *class > 3) {
		fmt.Fprintln(stderr, "recalls: --class must be 1, 2, or 3")
		return 2
	}

	src, ok := getSource("openfda_device_enforcement")
	if !ok {
		fmt.Fprintln(stderr, "recalls: enforcement source unavailable")
		return 1
	}
	recs, _, err := src.Fetch(ctx, sources.Query{Term: fs.Arg(0), Class: *class, Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "recalls: %v\n", err)
		return 1
	}

	meta := cliutil.Meta{Legend: cliutil.FDAClassLegend, EmptyMsg: cliutil.NoRecordsMsg}
	if err := cliutil.Output(stdout, stderr, enforcementRows(recs), meta, *f); err != nil {
		fmt.Fprintf(stderr, "recalls: %v\n", err)
		return 1
	}
	return 0
}
