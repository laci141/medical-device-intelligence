package sources

import (
	"strings"
	"testing"
)

func TestCTGovParams(t *testing.T) {
	s := NewClinicalTrials()

	v, err := s.params(Query{Term: "pacemaker", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("query.intr"); got != "pacemaker" {
		t.Errorf("query.intr=%q want pacemaker", got)
	}
	if got := v.Get("filter.advanced"); got != "AREA[InterventionType]DEVICE" {
		t.Errorf("device filter missing, got %q", got)
	}
	if got := v.Get("pageSize"); got != "5" {
		t.Errorf("pageSize=%q want 5", got)
	}
	if got := v.Get("countTotal"); got != "true" {
		t.Errorf("countTotal=%q want true", got)
	}

	v, err = s.params(Query{Term: "stent", Firm: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("query.spons"); got != "Acme" {
		t.Errorf("query.spons=%q want Acme", got)
	}
}

func TestCTGovParamsRejections(t *testing.T) {
	s := NewClinicalTrials()
	if _, err := s.params(Query{}); err == nil {
		t.Error("empty term must error")
	}
	// v2 paginates with page tokens; skip must be rejected, never silently ignored.
	if _, err := s.params(Query{Term: "x", Skip: 10}); err == nil {
		t.Error("skip pagination must be rejected explicitly")
	}
}

// Sample trimmed from a real v2 response (verified live 2026-07-09).
const ctgovSample = `{"totalCount":483,"studies":[
{"protocolSection":{"identificationModule":{"nctId":"NCT05252702","briefTitle":"Aveir DR i2i Study"},
"statusModule":{"overallStatus":"COMPLETED"},
"conditionsModule":{"conditions":["Cardiac Pacemaker, Artificial","Bradycardia"]},
"designModule":{"phases":["NA"]},
"armsInterventionsModule":{"interventions":[{"name":"Aveir DR Leadless Pacemaker System"}]}}}
],"nextPageToken":"abc"}`

func TestParseCTGov(t *testing.T) {
	recs, page, err := parseCTGov([]byte(ctgovSample))
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 483 || page.Returned != 1 {
		t.Errorf("page=%+v want total 483 returned 1", page)
	}
	r := recs[0]
	if r.ID != "NCT05252702" || r.Raw["nct_id"] != "NCT05252702" {
		t.Errorf("nct id not extracted: %+v", r)
	}
	if r.Raw["status"] != "COMPLETED" || r.Raw["phase"] != "NA" {
		t.Errorf("status/phase wrong: %+v", r.Raw)
	}
	if got := r.Raw["conditions"].(string); !strings.Contains(got, "Bradycardia") {
		t.Errorf("conditions not joined: %q", got)
	}
	if got := r.Raw["interventions"].(string); !strings.Contains(got, "Leadless Pacemaker") {
		t.Errorf("interventions not joined: %q", got)
	}
}

func TestParseCTGovEmptyIsNoMatchNotError(t *testing.T) {
	recs, page, err := parseCTGov([]byte(`{"totalCount":0,"studies":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 || page.Total != 0 {
		t.Errorf("want empty result, got %d recs total %d", len(recs), page.Total)
	}
}

func TestClinicalTrialsRegisteredLive(t *testing.T) {
	src, ok := Get("clinicaltrials")
	if !ok {
		t.Fatal("clinicaltrials must self-register")
	}
	if src.IDField() != "nct_id" {
		t.Errorf("IDField=%q want nct_id", src.IDField())
	}
	// The live adapter must have replaced the staged placeholder in the registry.
	if _, isStaged := src.(stagedSource); isStaged {
		t.Error("clinicaltrials is still the staged placeholder")
	}
}
