package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("export", cmdExport) }

// cmdExport dumps the local cache as CSV (default) or JSON. CSV rows stay
// machine-clean on stdout with the disclaimer on stderr (guardrail 9); --out
// writes the same payload to a file instead.
func cmdExport(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("export")
	db := fs.String("db", defaultDBPath(), "path to the SQLite cache")
	format := fs.String("format", "csv", "output format: csv | json")
	out := fs.String("out", "", "write to this file instead of stdout")
	if err := parse(fs, stderr, args, map[string]bool{"db": true, "format": true, "out": true}); err != nil {
		return 2
	}
	if *format != "csv" && *format != "json" {
		fmt.Fprintln(stderr, "export: --format must be 'csv' or 'json'")
		return 2
	}

	if _, err := os.Stat(*db); err != nil {
		fmt.Fprintf(stderr, "export: cache not found at %s; run sync first\n", *db)
		return 1
	}
	st, err := openStoreAt(*db)
	if err != nil {
		fmt.Fprintf(stderr, "export: open cache: %v\n", err)
		return 1
	}
	defer st.Close()

	all, err := st.AllRecords()
	if err != nil {
		fmt.Fprintf(stderr, "export: %v\n", err)
		return 1
	}
	rows := make([]map[string]any, 0, len(all))
	for _, r := range all {
		rows = append(rows, map[string]any{
			"source":     r.Source,
			"record_id":  r.RecordID,
			"term":       r.Term,
			"date":       r.Date,
			"summary":    r.Summary,
			"fetched_at": r.FetchedAt,
		})
	}

	// The shared --json/--csv/--agent flags win if set; otherwise --format picks.
	if !f.JSON && !f.Agent && !f.CSV {
		switch *format {
		case "csv":
			f.CSV = true
		case "json":
			f.JSON = true
		}
	}

	dest := stdout
	if *out != "" {
		file, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(stderr, "export: %v\n", err)
			return 1
		}
		defer file.Close()
		dest = file
	}

	meta := cliutil.Meta{
		Summary: []cliutil.KV{
			{Key: "records", Value: len(rows)},
			{Key: "exported_at", Value: time.Now().UTC().Format(time.RFC3339)},
		},
		EmptyMsg: "no records in local cache; run sync first",
	}
	if err := cliutil.Output(dest, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "export: %v\n", err)
		return 1
	}
	if *out != "" {
		fmt.Fprintf(stdout, "exported %d records to %s\n", len(rows), *out)
	}
	return 0
}
