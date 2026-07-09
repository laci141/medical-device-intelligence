package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sampleRecords() []map[string]any {
	return []map[string]any{
		{"recall_number": "Z-1234-2024", "classification": "Class II", "firm": "Acme"},
	}
}

// TestPlainKeepsDisclaimerAndLegend is the guardrail-8 regression: plain output
// (the piped default, since we never inspect the TTY) must carry the legend and
// the disclaimer.
func TestPlainKeepsDisclaimerAndLegend(t *testing.T) {
	var out, errBuf bytes.Buffer
	meta := Meta{Legend: FDAClassLegend}
	if err := Output(&out, &errBuf, sampleRecords(), meta, Flags{}); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, Disclaimer) {
		t.Error("plain output must contain the disclaimer")
	}
	if !strings.Contains(s, FDAClassLegend) {
		t.Error("plain output must contain the legend")
	}
	if !strings.Contains(s, "Z-1234-2024") {
		t.Error("plain output must contain the record id")
	}
}

// TestMachinePathOnlyOnExplicitFlag proves the envelope is emitted only for
// --json/--agent, and that even then it carries the disclaimer as a field.
func TestMachinePathOnlyOnExplicitFlag(t *testing.T) {
	for _, f := range []Flags{{JSON: true}, {Agent: true}} {
		var out, errBuf bytes.Buffer
		if err := Output(&out, &errBuf, sampleRecords(), Meta{}, f); err != nil {
			t.Fatal(err)
		}
		var env map[string]any
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("machine output must be valid JSON: %v", err)
		}
		if env["disclaimer"] != Disclaimer {
			t.Error("machine envelope must carry the disclaimer field")
		}
	}
}

// TestCSVDisclaimerToStderr proves CSV rows stay on stdout while the disclaimer
// goes to stderr (guardrail 9).
func TestCSVDisclaimerToStderr(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := Output(&out, &errBuf, sampleRecords(), Meta{Legend: FDAClassLegend}, Flags{CSV: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), Disclaimer) {
		t.Error("CSV stdout must NOT contain the disclaimer (keep rows machine-clean)")
	}
	if !strings.Contains(errBuf.String(), Disclaimer) {
		t.Error("CSV disclaimer must go to stderr")
	}
	if !strings.Contains(out.String(), "recall_number") {
		t.Error("CSV stdout must contain the header row")
	}
}

// TestEmptyNeverSaysSafe proves the empty result path says "no records found"
// and prints the disclaimer, never implying safety.
func TestEmptyNeverSaysSafe(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := Output(&out, &errBuf, nil, Meta{}, Flags{}); err != nil {
		t.Fatal(err)
	}
	s := strings.ToLower(out.String())
	if !strings.Contains(s, "no records found") {
		t.Error("empty plain output must say 'no records found'")
	}
	// The guardrail is "never AFFIRM the subject is safe" — not "never use the
	// word 'safety'". Our disclaimer correctly says it is NOT a safety verdict,
	// so we check for affirmative safety CLAIMS, which must never appear.
	for _, banned := range []string{"is safe", "are safe", "safe to use", "appears safe", "no known risk"} {
		if strings.Contains(s, banned) {
			t.Errorf("output must never affirm safety; found %q", banned)
		}
	}
}
