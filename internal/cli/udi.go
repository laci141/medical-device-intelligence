package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("udi", cmdUDI) }

// cmdUDI resolves a UDI-DI (or brand name) to a GUDID device profile via the
// live openFDA device/udi source. Cites the public_device_record_key.
func cmdUDI(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("udi")
	limit := fs.Int("limit", 5, "max profiles (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "udi: a UDI-DI or brand name is required, e.g. udi 00801741024785")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "udi: --limit must be >= 1")
		return 2
	}

	src, ok := getSource("openfda_device_udi")
	if !ok {
		fmt.Fprintln(stderr, "udi: GUDID source unavailable")
		return 1
	}
	recs, _, err := src.Fetch(ctx, sources.Query{Term: fs.Arg(0), Limit: *limit})
	if err != nil {
		fmt.Fprintf(stderr, "udi: %v\n", err)
		return 1
	}

	rows := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, map[string]any{
			"record_key":  r.ID,
			"brand_name":  str(r.Raw["brand_name"]),
			"company":     str(r.Raw["company_name"]),
			"description": clip(str(r.Raw["device_description"]), 140),
			"version":     str(r.Raw["version_or_model_number"]),
		})
	}
	meta := cliutil.Meta{EmptyMsg: cliutil.NoRecordsMsg}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "udi: %v\n", err)
		return 1
	}
	return 0
}
