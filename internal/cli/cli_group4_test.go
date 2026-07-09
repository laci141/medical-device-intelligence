package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/sources"
)

// fakeCountingSource is a Source whose Fetch answers from a function, so tests
// can vary totals by query (date windows, class filters).
type fakeCountingSource struct {
	name  string
	id    string
	fetch func(q sources.Query) ([]sources.RawRecord, sources.Page, error)
}

func (f fakeCountingSource) Name() string    { return f.name }
func (f fakeCountingSource) IDField() string { return f.id }
func (f fakeCountingSource) Fetch(_ context.Context, q sources.Query) ([]sources.RawRecord, sources.Page, error) {
	return f.fetch(q)
}
func (f fakeCountingSource) Health(context.Context) error { return nil }

// fakeFieldCounter adds the sources.FieldCounter capability.
type fakeFieldCounter struct {
	fakeSource
	counts map[string]int
}

func (f fakeFieldCounter) CountField(context.Context, sources.Query, string) (map[string]int, error) {
	return f.counts, nil
}

func group4Sources() map[string]sources.Source {
	return map[string]sources.Source{
		"openfda_device_enforcement": fakeSource{name: "openfda_device_enforcement", id: "recall_number",
			recs: []sources.RawRecord{enforcementRec()}},
		"openfda_device_event": fakeEventSource{
			fakeSource: fakeSource{name: "openfda_device_event", id: "mdr_report_key",
				recs: []sources.RawRecord{eventRec("M-1", "Death", "20240601")}},
			counts: map[string]int{"Death": 9, "Injury": 90, "Malfunction": 900},
		},
		"clinicaltrials": fakeSource{name: "clinicaltrials", id: "nct_id",
			recs: []sources.RawRecord{trialRec("NCT-1", "Trial A", "RECRUITING")}},
		"pubmed": fakeSource{name: "pubmed", id: "pmid",
			recs: []sources.RawRecord{pubRec("P-1", "Paper B", "2025")}},
		"openfda_device_udi": fakeSource{name: "openfda_device_udi", id: "public_device_record_key",
			recs: []sources.RawRecord{{ID: "key-1", Raw: map[string]any{"brand_name": "CardioX", "company_name": "Acme"}}}},
	}
}

func TestDeviceReportComposite(t *testing.T) {
	withSources(t, group4Sources())
	out, _, code := run(cmdDeviceReport, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{
		"recalls_total", "serious_adverse_events", "deaths", "trials_total",
		"publications_total", "regulatory_coverage",
		"key-1", "Z-1-2024", "M-1", "NCT-1", "P-1",
		"identity", "recall", "serious_event", "trial", "publication",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("device-report missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "not medical advice") {
		t.Error("device-report must carry the disclaimer")
	}
}

func TestCompareSideBySide(t *testing.T) {
	withSources(t, group4Sources())
	out, _, code := run(cmdCompare, "pacemaker", "stent")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"pacemaker", "stent", "recalls_total", "deaths", "trials_total", "not risk"} {
		if !strings.Contains(out, want) {
			t.Errorf("compare missing %q\n%s", want, out)
		}
	}
}

func TestCompareUsageErrors(t *testing.T) {
	withSources(t, group4Sources())
	if _, _, code := run(cmdCompare, "pacemaker"); code != 2 {
		t.Errorf("one device exit=%d want 2", code)
	}
	if _, errStr, code := run(cmdCompare, "pacemaker", "pacemaker"); code != 2 || !strings.Contains(errStr, "differ") {
		t.Errorf("same device exit=%d want 2 with message", code)
	}
}

func TestEmergingWindows(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { nowFunc = old })

	// Totals differ by window: the recent window (from 20250709) returns 7,
	// the prior window returns 3.
	byWindow := func(q sources.Query) ([]sources.RawRecord, sources.Page, error) {
		if q.DateFrom == "20250709" {
			return nil, sources.Page{Total: 7}, nil
		}
		return nil, sources.Page{Total: 3}, nil
	}
	withSources(t, map[string]sources.Source{
		"openfda_device_enforcement": fakeCountingSource{name: "openfda_device_enforcement", id: "recall_number", fetch: byWindow},
		"openfda_device_event":       fakeCountingSource{name: "openfda_device_event", id: "mdr_report_key", fetch: byWindow},
	})
	out, _, code := run(cmdEmerging, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{
		"20250709 to 20260709", "20240709 to 20250708",
		"recalls", "serious_adverse_events", "7", "3",
		"reporting lag", "not evidence of improvement",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("emerging missing %q\n%s", want, out)
		}
	}
}

func TestScoreTransparentFormula(t *testing.T) {
	withSources(t, group4Sources())
	out, _, code := run(cmdScore, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	// serious=99 -> 10*log10(100)=20.0; deaths=9 -> 15*log10(10)=15.0;
	// recalls=1 -> 3.0; class1=1 -> 4.5 (fakeSource ignores the class filter).
	for _, want := range []string{
		"signal_volume_index", "42.5", "formula", "log10",
		"NOT a risk or safety score", "20.0", "15.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("score missing %q\n%s", want, out)
		}
	}
}

func TestAnalyticsDistributionsSorted(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"openfda_device_event": fakeEventSource{
			fakeSource: fakeSource{name: "openfda_device_event", id: "mdr_report_key"},
			counts:     map[string]int{"Death": 5, "Malfunction": 500, "Injury": 50},
		},
		"openfda_device_enforcement": fakeFieldCounter{
			fakeSource: fakeSource{name: "openfda_device_enforcement", id: "recall_number"},
			counts:     map[string]int{"Class II": 164, "Class I": 31, "Class III": 2},
		},
	})
	out, _, code := run(cmdAnalytics, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"maude_event_type", "recall_classification", "Malfunction", "Class II", "164"} {
		if !strings.Contains(out, want) {
			t.Errorf("analytics missing %q\n%s", want, out)
		}
	}
	// Largest-first within each dimension.
	if strings.Index(out, "Malfunction") > strings.Index(out, "Death") {
		t.Error("event types must sort largest first")
	}
	if strings.Index(out, "Class II") > strings.Index(out, "Class III") {
		t.Error("recall classes must sort largest first")
	}
}

func TestGroup4CommandsRegistered(t *testing.T) {
	for _, name := range []string{"device-report", "compare", "emerging", "score", "analytics"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}
