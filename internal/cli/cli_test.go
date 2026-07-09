package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/laci141/medical-device-intelligence/internal/sources"
)

// fakeSource is a hermetic stand-in for a live provider.
type fakeSource struct {
	name string
	id   string
	recs []sources.RawRecord
	err  error
}

func (f fakeSource) Name() string    { return f.name }
func (f fakeSource) IDField() string { return f.id }
func (f fakeSource) Fetch(context.Context, sources.Query) ([]sources.RawRecord, sources.Page, error) {
	if f.err != nil {
		return nil, sources.Page{}, f.err
	}
	return f.recs, sources.Page{Total: len(f.recs), Returned: len(f.recs)}, nil
}
func (f fakeSource) Health(context.Context) error { return f.err }

// withSources installs fake sources for the duration of a test.
func withSources(t *testing.T, m map[string]sources.Source) {
	t.Helper()
	oldGet, oldAll := getSource, allSources
	getSource = func(name string) (sources.Source, bool) { s, ok := m[name]; return s, ok }
	allSources = func() []string {
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)
		return names
	}
	t.Cleanup(func() { getSource, allSources = oldGet, oldAll })
}

func enforcementRec() sources.RawRecord {
	return sources.RawRecord{ID: "Z-1-2024", Raw: map[string]any{
		"classification": "Class II", "recalling_firm": "Acme",
		"product_description": "pacemaker model X", "reason_for_recall": "battery issue",
		"recall_initiation_date": "20240101",
	}}
}

func run(h Handler, args ...string) (string, string, int) {
	var out, errBuf bytes.Buffer
	code := h(context.Background(), &out, &errBuf, args)
	return out.String(), errBuf.String(), code
}

func TestRecallsHappyPathCitesAndDisclaims(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number", recs: []sources.RawRecord{enforcementRec()}},
	})
	out, _, code := run(cmdRecalls, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out, "Z-1-2024") {
		t.Error("must cite recall_number")
	}
	if !strings.Contains(out, "not medical advice") {
		t.Error("plain output must carry the disclaimer")
	}
}

func TestRecallsJSONHasDisclaimerField(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number", recs: []sources.RawRecord{enforcementRec()}},
	})
	// Flags placed AFTER the positional term must still bind (ReorderArgs).
	out, _, code := run(cmdRecalls, "pacemaker", "--limit", "1", "--json")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("--json must emit valid JSON: %v\n%s", err, out)
	}
	if _, ok := env["disclaimer"]; !ok {
		t.Error("JSON envelope must carry a disclaimer field")
	}
}

func TestRecallsBadClassExit2(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number"},
	})
	_, errStr, code := run(cmdRecalls, "pacemaker", "--class", "9")
	if code != 2 {
		t.Fatalf("bad --class exit=%d want 2", code)
	}
	if !strings.Contains(errStr, "--class must be 1, 2, or 3") {
		t.Errorf("expected class validation message, got %q", errStr)
	}
}

func TestRecallsMissingTermExit2(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number"},
	})
	if _, _, code := run(cmdRecalls); code != 2 {
		t.Fatalf("missing term exit=%d want 2", code)
	}
}

func TestNoMatchSaysNoRecordsNotSafe(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number", recs: nil},
	})
	out, _, code := run(cmdRecalls, "zzznope")
	if code != 0 {
		t.Fatalf("no-match exit=%d want 0", code)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "no records found") {
		t.Error("must say 'no records found'")
	}
	for _, banned := range []string{"is safe", "are safe", "safe to use"} {
		if strings.Contains(low, banned) {
			t.Errorf("must never affirm safety; found %q", banned)
		}
	}
}

func TestSearchAggregatesAndNotesStaged(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number", recs: []sources.RawRecord{enforcementRec()}},
		"clinicaltrials":             fakeSource{name: "clinicaltrials", id: "nct_id", err: sources.ErrNotWired},
	})
	out, errStr, code := run(cmdSearch, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out, "Z-1-2024") {
		t.Error("aggregated result must cite the source id")
	}
	if !strings.Contains(errStr, "staged") || !strings.Contains(errStr, "clinicaltrials") {
		t.Errorf("staged source must be noted on stderr, got %q", errStr)
	}
}

func TestManufacturersFirmFilter(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number", recs: []sources.RawRecord{enforcementRec()}},
	})
	out, _, code := run(cmdManufacturers, "Acme")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out, "Z-1-2024") || !strings.Contains(out, "Acme") {
		t.Error("must list the firm's recalls with ids")
	}
}

func TestUDIProfile(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_udi": fakeSource{name: "openfda_device_udi", id: "public_device_record_key",
			recs: []sources.RawRecord{{ID: "key-1", Raw: map[string]any{"brand_name": "CardioX", "company_name": "Acme"}}}},
	})
	out, _, code := run(cmdUDI, "CardioX")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out, "key-1") || !strings.Contains(out, "CardioX") {
		t.Error("udi profile must cite the record key and show the brand")
	}
}

func TestSummaryBreakdown(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number",
			recs: []sources.RawRecord{enforcementRec(), {ID: "Z-2-2024", Raw: map[string]any{"classification": "Class I"}}}},
	})
	out, _, code := run(cmdSummary, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out, "Class I") || !strings.Contains(out, "Class II") {
		t.Error("summary must break down by classification")
	}
	if !strings.Contains(out, "not a safety verdict") {
		t.Error("summary must carry the disclaimer")
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Dispatch(context.Background(), &out, &errBuf, []string{"nope"}); code != 2 {
		t.Fatalf("unknown command exit=%d want 2", code)
	}
	if !strings.Contains(errBuf.String(), "unknown command") {
		t.Error("expected 'unknown command' message")
	}
}

func TestAllGroup1CommandsRegistered(t *testing.T) {
	for _, name := range []string{"search", "udi", "manufacturers", "summary", "recalls", "doctor"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}
