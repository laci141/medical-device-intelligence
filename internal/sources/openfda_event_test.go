package sources

import (
	"strings"
	"testing"
)

func TestEventBuildSearchSeverity(t *testing.T) {
	s := NewOpenFDADeviceEvent()

	all, err := s.buildSearch(Query{Term: "pacemaker"})
	if err != nil {
		t.Fatal(err)
	}
	want := `(device.generic_name:"pacemaker" OR device.brand_name:"pacemaker")`
	if all != want {
		t.Fatalf("all=%q want %q", all, want)
	}

	death, _ := s.buildSearch(Query{Term: "pacemaker", Severity: "death"})
	if !strings.Contains(death, `AND event_type:"Death"`) {
		t.Fatalf("death filter missing: %q", death)
	}

	serious, _ := s.buildSearch(Query{Term: "pacemaker", Severity: "serious"})
	if !strings.Contains(serious, `(event_type:"Death" OR event_type:"Injury")`) {
		t.Fatalf("serious filter wrong: %q", serious)
	}
	// Guardrail 2: literal spaces on the wire, never +AND+/+OR+.
	if strings.Contains(serious, "+AND+") || strings.Contains(serious, "+OR+") {
		t.Fatalf("must use literal spaces: %q", serious)
	}
}

func TestEventBuildSearchDateRange(t *testing.T) {
	s := NewOpenFDADeviceEvent()
	got, err := s.buildSearch(Query{Term: "pacemaker", DateFrom: "20250709", DateTo: "20260709"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "date_received:[20250709 TO 20260709]") {
		t.Fatalf("date range clause missing: %q", got)
	}
}

func TestEnforcementImplementsFieldCounter(t *testing.T) {
	src, ok := Get("openfda_device_enforcement")
	if !ok {
		t.Fatal("enforcement source must self-register")
	}
	if _, ok := src.(FieldCounter); !ok {
		t.Error("enforcement source must implement FieldCounter")
	}
}

func TestEventBuildSearchRejectsBadSeverity(t *testing.T) {
	s := NewOpenFDADeviceEvent()
	if _, err := s.buildSearch(Query{Term: "x", Severity: "kinda-bad"}); err == nil {
		t.Fatal("invalid severity must error")
	}
	if _, err := s.buildSearch(Query{Severity: "death"}); err == nil {
		t.Fatal("empty term must error")
	}
}

func TestEventRegisteredWithIDField(t *testing.T) {
	src, ok := Get("openfda_device_event")
	if !ok {
		t.Fatal("event source must self-register")
	}
	if src.IDField() != "mdr_report_key" {
		t.Errorf("event IDField=%q want mdr_report_key", src.IDField())
	}
	if _, ok := src.(EventCounter); !ok {
		t.Error("event source must implement EventCounter")
	}
}
