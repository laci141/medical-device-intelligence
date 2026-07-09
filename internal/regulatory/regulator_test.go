package regulatory

import "testing"

func TestFDAIsLiveOthersAreSkeletons(t *testing.T) {
	fda, ok := Get("FDA")
	if !ok {
		t.Fatal("FDA must be registered")
	}
	if !fda.Available() {
		t.Error("FDA adapter should report Available()==true (openFDA is live)")
	}
	for _, agency := range []string{"EMA", "HealthCanada", "TGA", "PMDA"} {
		r, ok := Get(agency)
		if !ok {
			t.Fatalf("%s must be registered as a skeleton", agency)
		}
		if r.Available() {
			t.Errorf("%s should be a skeleton (Available()==false) until a keyless API exists", agency)
		}
	}
}

func TestAllAgenciesRegistered(t *testing.T) {
	if len(Agencies()) != 5 {
		t.Fatalf("expected 5 agencies, got %v", Agencies())
	}
}
