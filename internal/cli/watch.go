package cli

import (
	"context"
	"fmt"
	"io"
	"time"
)

func init() { register("watch", cmdWatch) }

// watchWait pauses between polls; an indirection so tests can skip real time.
// It returns false when the context was canceled (Ctrl+C) during the wait.
var watchWait = func(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// cmdWatch keeps the local cache fresh: an initial full sync, then a poll
// loop that fetches deltas since the last recorded sync. The interval floor
// (60s) protects the keyless API rate limits; Ctrl+C stops the loop cleanly
// between or during waits.
func cmdWatch(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("watch")
	_ = f // watch streams plain progress lines; machine modes apply per-sync via `sync`
	db := fs.String("db", defaultDBPath(), "path to the SQLite cache")
	interval := fs.Int("interval", 3600, "seconds between polls (>=60)")
	polls := fs.Int("polls", 0, "stop after N polls (0 = run until Ctrl+C)")
	batch := fs.Int("batch", 100, "upsert batch size per transaction (>=1)")
	limit := fs.Int("limit", 50, "max records per source per poll (>=1)")
	if err := parse(fs, stderr, args, map[string]bool{"db": true, "interval": true, "polls": true, "batch": true, "limit": true}); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "watch: a device name is required, e.g. watch pacemaker")
		return 2
	}
	if *interval < 60 {
		fmt.Fprintln(stderr, "watch: --interval must be >= 60 seconds (API rate limits)")
		return 2
	}
	if *polls < 0 {
		fmt.Fprintln(stderr, "watch: --polls must be >= 0")
		return 2
	}
	if *batch < 1 || *limit < 1 {
		fmt.Fprintln(stderr, "watch: --batch and --limit must be >= 1")
		return 2
	}
	device := fs.Arg(0)

	st, err := openStoreAt(*db)
	if err != nil {
		fmt.Fprintf(stderr, "watch: open cache: %v\n", err)
		return 1
	}
	defer st.Close()

	fmt.Fprintf(stdout, "watch started: device=%s db=%s interval=%ds (Ctrl+C to stop)\n",
		device, *db, *interval)

	for i := 1; ; i++ {
		// Delta window: since the last recorded sync for this term; the first
		// ever pass has no history and syncs without a window.
		since := ""
		if ts, ok, err := st.LastSyncTime(device); err != nil {
			fmt.Fprintf(stderr, "watch: last sync time: %v\n", err)
		} else if ok {
			since = rfc3339ToCompact(ts)
		}
		until := time.Now().UTC().Format("20060102")

		res, err := syncPass(ctx, stderr, st, device, since, until, *batch, *limit)
		if err != nil {
			fmt.Fprintf(stderr, "watch: sync %d: %v\n", i, err)
			return 1
		}
		fmt.Fprintf(stdout, "sync %d: %d fetched, %d new (cache total %d)\n",
			i, res.Fetched, res.New, res.Total)

		if *polls > 0 && i >= *polls {
			fmt.Fprintf(stdout, "watch done after %d poll(s)\n", i)
			return 0
		}
		next := time.Now().Add(time.Duration(*interval) * time.Second)
		fmt.Fprintf(stdout, "next sync at %s\n", next.Format(time.RFC3339))
		if !watchWait(ctx, time.Duration(*interval)*time.Second) {
			fmt.Fprintln(stdout, "watch stopped")
			return 0
		}
	}
}
