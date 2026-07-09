package sources

import (
	"strings"
	"testing"
)

func TestUDIBuildSearch(t *testing.T) {
	s := NewOpenFDADeviceUDI()
	got := s.buildSearch(Query{Term: "00801741024785"})
	want := `identifiers.id:"00801741024785" OR brand_name:"00801741024785"`
	if got != want {
		t.Fatalf("udi buildSearch=%q want %q", got, want)
	}
	if s.buildSearch(Query{}) != "" {
		t.Error("empty term should yield empty search")
	}
}

func TestUDIRegisteredAndIDField(t *testing.T) {
	src, ok := Get("openfda_device_udi")
	if !ok {
		t.Fatal("udi source must self-register")
	}
	if src.IDField() != "public_device_record_key" {
		t.Errorf("udi IDField=%q", src.IDField())
	}
}

func TestParseUDI(t *testing.T) {
	body := []byte(`{"meta":{"results":{"total":1}},"results":[
		{"public_device_record_key":"abc-123","brand_name":"CardioX","company_name":"Acme"}]}`)
	recs, page, err := parseUDI(body, "public_device_record_key")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(recs) != 1 || recs[0].ID != "abc-123" {
		t.Fatalf("parseUDI got total=%d n=%d id=%q", page.Total, len(recs), recs[0].ID)
	}
}

func TestEnforcementFirmSearch(t *testing.T) {
	s := NewOpenFDADeviceEnforcement()
	got, err := s.buildSearch(Query{Firm: "Medtronic"})
	if err != nil {
		t.Fatal(err)
	}
	want := `recalling_firm:"Medtronic"`
	if got != want {
		t.Fatalf("firm search=%q want %q", got, want)
	}
	// Term + Firm combine with AND (literal spaces).
	both, _ := s.buildSearch(Query{Term: "pump", Firm: "Acme"})
	if !strings.Contains(both, `product_description:"pump" AND recalling_firm:"Acme"`) {
		t.Fatalf("combined search wrong: %q", both)
	}
}
