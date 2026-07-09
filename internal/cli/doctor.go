package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/regulatory"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("doctor", cmdDoctor) }

// cmdDoctor probes each live source for reachability + latency and reports the
// staged sources and skeleton regulators so the user sees exactly what is wired.
func cmdDoctor(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("doctor")
	if err := parse(fs, stderr, args, nil); err != nil {
		return 2
	}

	// Probe every registered source. Live vs staged is derived from Health:
	// a staged adapter reports ErrNotWired, so the list can never drift from
	// the registry the way a hardcoded live-source list did.
	names := allSources()
	sort.Strings(names)
	rows := make([]map[string]any, 0, len(names))
	worstErr := false
	for _, name := range names {
		src, ok := getSource(name)
		if !ok {
			continue
		}
		start := time.Now()
		err := src.Health(ctx)
		lat := time.Since(start).Round(time.Millisecond)
		if errors.Is(err, sources.ErrNotWired) {
			rows = append(rows, map[string]any{
				"source": name, "status": "staged (not yet integrated)", "wired": false,
			})
			continue
		}
		status := "reachable"
		if err != nil {
			status = "UNREACHABLE: " + err.Error()
			worstErr = true
		}
		rows = append(rows, map[string]any{
			"source":     name,
			"status":     status,
			"latency_ms": lat.Milliseconds(),
			"wired":      true,
		})
	}

	// Regulator adapters and their availability.
	for _, r := range regulatory.All() {
		rows = append(rows, map[string]any{
			"source": "regulator:" + r.Agency(), "status": availStr(r.Available()),
			"jurisdiction": r.Jurisdiction(), "wired": r.Available(),
		})
	}

	meta := cliutil.Meta{}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "doctor: %v\n", err)
		return 1
	}
	if worstErr {
		return 1
	}
	return 0
}

func availStr(a bool) string {
	if a {
		return "live"
	}
	return "skeleton (awaiting keyless API)"
}
