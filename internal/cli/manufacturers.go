package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("manufacturers", cmdManufacturers) }

// cmdManufacturers lists recalls attributed to a recalling firm / manufacturer,
// via the live openFDA enforcement source. Cites recall_number on every row.
func cmdManufacturers(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("manufacturers")
	limit := fs.Int("limit", 25, "max records (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "manufacturers: a firm name is required, e.g. manufacturers Medtronic")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "manufacturers: --limit must be >= 1")
		return 2
	}

	src, ok := getSource("openfda_device_enforcement")
	if !ok {
		fmt.Fprintln(stderr, "manufacturers: enforcement source unavailable")
		return 1
	}
	recs, _, err := src.Fetch(ctx, sources.Query{Firm: fs.Arg(0), Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "manufacturers: %v\n", err)
		return 1
	}

	meta := cliutil.Meta{Legend: cliutil.FDAClassLegend, EmptyMsg: cliutil.NoRecordsMsg}
	if err := cliutil.Output(stdout, stderr, enforcementRows(recs), meta, *f); err != nil {
		fmt.Fprintf(stderr, "manufacturers: %v\n", err)
		return 1
	}
	return 0
}
