package sources

import (
	"strings"
	"testing"
)

func TestOpenFDABuildSearch(t *testing.T) {
	s := NewOpenFDADeviceEnforcement()

	got, err := s.buildSearch(Query{Term: "pacemaker", Class: 2,
		DateFrom: "20240101", DateTo: "20241231"})
	if err != nil {
		t.Fatal(err)
	}
	want := `product_description:"pacemaker" AND classification:"Class II" AND recall_initiation_date:[20240101 TO 20241231]`
	if got != want {
		t.Fatalf("buildSearch=\n %q\nwant\n %q", got, want)
	}
	if strings.Contains(got, "+AND+") || strings.Contains(got, "+TO+") {
		t.Fatalf("search must use literal spaces, not +AND+/+TO+: %q", got)
	}
}

func TestOpenFDABuildSearchRejectsBadClass(t *testing.T) {
	s := NewOpenFDADeviceEnforcement()
	if _, err := s.buildSearch(Query{Term: "x", Class: 9}); err == nil {
		t.Fatal("class 9 must be rejected")
	}
}

func TestParseEnforcement(t *testing.T) {
	body := []byte(`{"meta":{"results":{"total":2}},"results":[
		{"recall_number":"Z-1-2024","classification":"Class I"},
		{"recall_number":"Z-2-2024","classification":"Class II"}]}`)
	recs, page, err := parseEnforcement(body, "recall_number")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(recs) != 2 {
		t.Fatalf("got total=%d n=%d want 2/2", page.Total, len(recs))
	}
	if recs[0].ID != "Z-1-2024" {
		t.Errorf("id=%q want Z-1-2024", recs[0].ID)
	}
}

func TestOpenFDARegistered(t *testing.T) {
	if _, ok := Get("openfda_device_enforcement"); !ok {
		t.Fatal("openFDA adapter must self-register via init()")
	}
}
