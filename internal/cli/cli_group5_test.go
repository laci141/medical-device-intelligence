package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/store"
)

func tempDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mdi.db")
}

func TestSyncRoundTripIdempotent(t *testing.T) {
	withSources(t, group4Sources()) // 5 live fakes, one record each
	db := tempDB(t)

	out, _, code := run(cmdSync, "pacemaker", "--db", db)
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"records_fetched", "records_new", "records_in_cache"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q\n%s", want, out)
		}
	}

	// The cache must actually contain the rows (query back through the store).
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.CountRecords()
	st.Close()
	if err != nil || n != 5 {
		t.Fatalf("cache rows=%d err=%v want 5", n, err)
	}

	// Re-sync: idempotent — no new rows.
	out, _, code = run(cmdSync, "pacemaker", "--db", db, "--json")
	if code != 0 {
		t.Fatalf("re-sync exit=%d want 0", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("sync --json must emit valid JSON: %v", err)
	}
	sum, _ := env["summary"].(map[string]any)
	if got := sum["records_new"]; got != float64(0) {
		t.Errorf("re-sync records_new=%v want 0 (upsert must be idempotent)", got)
	}
}

func TestSyncUsageErrors(t *testing.T) {
	withSources(t, group4Sources())
	if _, _, code := run(cmdSync); code != 2 {
		t.Errorf("missing device exit=%d want 2", code)
	}
	if _, errStr, code := run(cmdSync, "pacemaker", "--since", "2025-01-01"); code != 2 || !strings.Contains(errStr, "--since") {
		t.Errorf("bad --since exit=%d want 2 with message", code)
	}
	if _, _, code := run(cmdSync, "pacemaker", "--batch", "0"); code != 2 {
		t.Errorf("bad --batch exit=%d want 2", code)
	}
}

func TestExportCSVKeepsStdoutClean(t *testing.T) {
	withSources(t, group4Sources())
	db := tempDB(t)
	if _, _, code := run(cmdSync, "pacemaker", "--db", db); code != 0 {
		t.Fatal("seed sync failed")
	}

	out, errStr, code := run(cmdExport, "--db", db, "--format", "csv")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 { // header + 5 rows
		t.Fatalf("csv lines=%d want 6:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "record_id") || !strings.Contains(lines[0], "source") {
		t.Errorf("csv header wrong: %q", lines[0])
	}
	if strings.Contains(out, "not medical advice") {
		t.Error("disclaimer must NOT pollute CSV stdout")
	}
	if !strings.Contains(errStr, "not medical advice") {
		t.Error("disclaimer must go to stderr in CSV mode")
	}
}

func TestExportJSONEnvelope(t *testing.T) {
	withSources(t, group4Sources())
	db := tempDB(t)
	if _, _, code := run(cmdSync, "pacemaker", "--db", db); code != 0 {
		t.Fatal("seed sync failed")
	}
	out, _, code := run(cmdExport, "--db", db, "--format", "json")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json export invalid: %v", err)
	}
	if _, ok := env["disclaimer"]; !ok {
		t.Error("json export must carry the disclaimer")
	}
	if recs, _ := env["records"].([]any); len(recs) != 5 {
		t.Errorf("json records=%d want 5", len(env["records"].([]any)))
	}
}

func TestExportErrors(t *testing.T) {
	if _, errStr, code := run(cmdExport, "--db", filepath.Join("Z:", "nope", "missing.db")); code != 1 || !strings.Contains(errStr, "run sync first") {
		t.Errorf("missing db exit=%d want 1 with hint", code)
	}
	if _, _, code := run(cmdExport, "--format", "xml"); code != 2 {
		t.Errorf("bad format exit=%d want 2", code)
	}
}

func TestWatchPollsBounded(t *testing.T) {
	withSources(t, group4Sources())
	db := tempDB(t)

	waits := 0
	oldWait := watchWait
	watchWait = func(context.Context, time.Duration) bool { waits++; return true }
	t.Cleanup(func() { watchWait = oldWait })

	out, _, code := run(cmdWatch, "pacemaker", "--db", db, "--polls", "2", "--interval", "60")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"watch started", "sync 1:", "sync 2:", "watch done after 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("watch output missing %q\n%s", want, out)
		}
	}
	if waits != 1 { // one wait between two polls, none after the last
		t.Errorf("waits=%d want 1", waits)
	}
}

func TestWatchStopsOnCancel(t *testing.T) {
	withSources(t, group4Sources())
	db := tempDB(t)

	oldWait := watchWait
	watchWait = func(context.Context, time.Duration) bool { return false } // canceled during wait
	t.Cleanup(func() { watchWait = oldWait })

	out, _, code := run(cmdWatch, "pacemaker", "--db", db, "--interval", "60")
	if code != 0 {
		t.Fatalf("cancel must exit 0 (graceful), got %d", code)
	}
	if !strings.Contains(out, "watch stopped") {
		t.Errorf("want graceful stop message:\n%s", out)
	}
}

func TestWatchIntervalFloor(t *testing.T) {
	withSources(t, group4Sources())
	if _, errStr, code := run(cmdWatch, "pacemaker", "--interval", "30"); code != 2 || !strings.Contains(errStr, ">= 60") {
		t.Errorf("interval floor exit=%d want 2 with message", code)
	}
}

func TestWorkflowList(t *testing.T) {
	out, _, code := run(cmdWorkflow, "--list")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"daily-sync", "compliance-check", "trend-watch"} {
		if !strings.Contains(out, want) {
			t.Errorf("workflow list missing %q\n%s", want, out)
		}
	}
}

func TestWorkflowRunDailySync(t *testing.T) {
	withSources(t, group4Sources())
	db := tempDB(t)
	out, errStr, code := run(cmdWorkflow, "--run", "daily-sync", "pacemaker", "--db", db)
	if code != 0 {
		t.Fatalf("exit=%d want 0\nstderr:\n%s", code, errStr)
	}
	for _, want := range []string{"[1/2] sync", "[2/2] export", "complete: 2 step(s), 0 errors"} {
		if !strings.Contains(errStr, want) {
			t.Errorf("progress (stderr) missing %q\n%s", want, errStr)
		}
	}
	// The export step's CSV lands on stdout, uncontaminated by progress lines.
	if !strings.Contains(out, "record_id") {
		t.Errorf("stdout should carry the exported CSV:\n%s", out)
	}
	if strings.Contains(out, "[2/2]") {
		t.Error("progress lines must not pollute stdout")
	}
}

func TestWorkflowFailFast(t *testing.T) {
	old := workflows
	workflows = append([]workflowDef{}, old...)
	workflows = append(workflows, workflowDef{
		name:        "broken",
		description: "test-only",
		steps: func(device, db string) [][]string {
			return [][]string{{"no-such-command"}, {"doctor"}}
		},
	})
	t.Cleanup(func() { workflows = old })

	_, errStr, code := run(cmdWorkflow, "--run", "broken", "x")
	if code == 0 {
		t.Fatal("failing step must fail the workflow")
	}
	if !strings.Contains(errStr, "FAILED") || !strings.Contains(errStr, "stopping workflow") {
		t.Errorf("want fail-fast message, got:\n%s", errStr)
	}
	if strings.Contains(errStr, "[2/2] doctor OK") {
		t.Error("steps after a failure must not run")
	}
}

func TestWorkflowUsageErrors(t *testing.T) {
	if _, _, code := run(cmdWorkflow); code != 2 {
		t.Errorf("no flags exit=%d want 2", code)
	}
	if _, errStr, code := run(cmdWorkflow, "--run", "nope", "x"); code != 2 || !strings.Contains(errStr, "unknown workflow") {
		t.Errorf("unknown workflow exit=%d want 2", code)
	}
	if _, _, code := run(cmdWorkflow, "--run", "daily-sync"); code != 2 {
		t.Errorf("missing device exit=%d want 2", code)
	}
}

func TestGroup5CommandsRegistered(t *testing.T) {
	for _, name := range []string{"sync", "watch", "export", "workflow"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}
