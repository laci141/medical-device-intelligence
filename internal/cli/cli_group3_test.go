package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/laci141/medical-device-intelligence/internal/sources"
)

// fakePubmedSource is a fakeSource that also implements sources.PMIDLookup.
type fakePubmedSource struct {
	fakeSource
	lookedUp []string // captured pmids
	lookup   []sources.RawRecord
}

func (f *fakePubmedSource) LookupPMIDs(_ context.Context, pmids []string) ([]sources.RawRecord, error) {
	f.lookedUp = pmids
	return f.lookup, nil
}

func trialRec(id, title, status string) sources.RawRecord {
	return sources.RawRecord{ID: id, Raw: map[string]any{
		"nct_id": id, "title": title, "status": status, "phase": "NA",
		"conditions": "Bradycardia", "interventions": "Leadless Pacemaker",
	}}
}

func pubRec(id, title, year string) sources.RawRecord {
	return sources.RawRecord{ID: id, Raw: map[string]any{
		"pmid": id, "title": title, "year": year, "journal": "Heart J", "doi": "10.1/x",
	}}
}

func TestTrialsCitesAndDisclaims(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"clinicaltrials": fakeSource{name: "clinicaltrials", id: "nct_id",
			recs: []sources.RawRecord{trialRec("NCT05252702", "Aveir DR i2i Study", "COMPLETED")}},
	})
	out, _, code := run(cmdTrials, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"NCT05252702", "trials_total", "COMPLETED", "not proof of efficacy"} {
		if !strings.Contains(out, want) {
			t.Errorf("trials output missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "not medical advice") {
		t.Error("trials must carry the disclaimer")
	}
}

func TestTrialsUsageErrors(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"clinicaltrials": fakeSource{name: "clinicaltrials", id: "nct_id"},
	})
	if _, _, code := run(cmdTrials); code != 2 {
		t.Errorf("missing term exit=%d want 2", code)
	}
	if _, errStr, code := run(cmdTrials, "pacemaker", "--limit", "0"); code != 2 || !strings.Contains(errStr, "--limit") {
		t.Errorf("bad limit exit=%d want 2 with message", code)
	}
}

func TestPublicationsSearch(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"pubmed": fakeSource{name: "pubmed", id: "pmid",
			recs: []sources.RawRecord{pubRec("42422410", "Fontan Circulation Study", "2026")}},
	})
	out, _, code := run(cmdPublications, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"42422410", "publications_total", "10.1/x", "not a quality or safety verdict"} {
		if !strings.Contains(out, want) {
			t.Errorf("publications output missing %q\n%s", want, out)
		}
	}
}

func TestPublicationsPMIDLookup(t *testing.T) {
	fake := &fakePubmedSource{
		fakeSource: fakeSource{name: "pubmed", id: "pmid"},
		lookup: []sources.RawRecord{
			pubRec("111", "Known Paper", "2020"),
			{ID: "999", Raw: map[string]any{"pmid": "999", "error": "cannot get document summary"}},
		},
	}
	withSources(t, map[string]sources.Source{"pubmed": fake})
	out, _, code := run(cmdPublications, "--pmid", "111,999")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if len(fake.lookedUp) != 2 || fake.lookedUp[0] != "111" {
		t.Errorf("pmids not passed to lookup: %v", fake.lookedUp)
	}
	if !strings.Contains(out, "pmids_requested") || !strings.Contains(out, "Known Paper") {
		t.Errorf("lookup output incomplete:\n%s", out)
	}
	// The unresolved pmid is named in the headline, never a mostly-empty row
	// and never silently dropped.
	if !strings.Contains(out, "pmids_unresolved") || !strings.Contains(out, "999") {
		t.Errorf("unresolved pmid must be named in the summary:\n%s", out)
	}
	if strings.Contains(out, "cannot get document summary") {
		t.Errorf("errored entry must not render as a record row:\n%s", out)
	}
}

func TestPublicationsUsageErrors(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"pubmed": fakeSource{name: "pubmed", id: "pmid"},
	})
	if _, _, code := run(cmdPublications); code != 2 {
		t.Errorf("no term/pmid exit=%d want 2", code)
	}
	if _, errStr, code := run(cmdPublications, "pacemaker", "--pmid", "1"); code != 2 || !strings.Contains(errStr, "not both") {
		t.Errorf("term+pmid conflict exit=%d want 2", code)
	}
}

func TestEvidenceComposite(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"clinicaltrials": fakeSource{name: "clinicaltrials", id: "nct_id",
			recs: []sources.RawRecord{trialRec("NCT-1", "Trial A", "RECRUITING")}},
		"pubmed": fakeSource{name: "pubmed", id: "pmid",
			recs: []sources.RawRecord{pubRec("P-1", "Paper B", "2025")}},
	})
	out, _, code := run(cmdEvidence, "pacemaker")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	for _, want := range []string{"trials_total", "publications_total", "NCT-1", "P-1", "not proof of efficacy or safety"} {
		if !strings.Contains(out, want) {
			t.Errorf("evidence output missing %q\n%s", want, out)
		}
	}
}

func TestEvidenceNoMatchNeverSafe(t *testing.T) {
	withSources(t, map[string]sources.Source{
		"clinicaltrials": fakeSource{name: "clinicaltrials", id: "nct_id"},
		"pubmed":         fakeSource{name: "pubmed", id: "pmid"},
	})
	out, _, code := run(cmdEvidence, "zzznope")
	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "no records found") {
		t.Errorf("want 'no records found', got:\n%s", out)
	}
	if strings.Contains(low, "is safe") || strings.Contains(low, "are safe") {
		t.Error("must never affirm safety")
	}
}

func TestGroup3CommandsRegistered(t *testing.T) {
	for _, name := range []string{"trials", "publications", "evidence"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}
