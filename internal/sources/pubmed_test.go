package sources

import (
	"strings"
	"testing"
)

func TestPubMedSearchTerm(t *testing.T) {
	got := searchTerm("insulin pump")
	if got != `"insulin pump"[Title/Abstract]` {
		t.Errorf("searchTerm=%q", got)
	}
}

// Sample trimmed from a real esummary response (verified live 2026-07-09).
const esummarySample = `{"header":{"type":"esummary"},"result":{
"uids":["42422410","999"],
"42422410":{"uid":"42422410","pubdate":"2026 Jun","source":"CJC Pediatr Congenit Heart Dis",
"fulljournalname":"CJC pediatric and congenital heart disease",
"title":"Exercise Capacity in Pediatric Patients With Fontan Circulation.",
"lastauthor":"Tierney S",
"articleids":[{"idtype":"pubmed","value":"42422410"},{"idtype":"doi","value":"10.1016/j.cjcpc.2025.10.010"}]},
"999":{"uid":"999","error":"cannot get document summary"}
}}`

func TestParseESummary(t *testing.T) {
	recs, err := parseESummary([]byte(esummarySample))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records (including the errored pmid), got %d", len(recs))
	}
	r := recs[0]
	if r.ID != "42422410" || r.Raw["pmid"] != "42422410" {
		t.Errorf("pmid not extracted: %+v", r)
	}
	if r.Raw["year"] != "2026" {
		t.Errorf("year=%v want 2026", r.Raw["year"])
	}
	if r.Raw["doi"] != "10.1016/j.cjcpc.2025.10.010" {
		t.Errorf("doi=%v", r.Raw["doi"])
	}
	if got := r.Raw["journal"].(string); !strings.Contains(got, "pediatric") {
		t.Errorf("journal should prefer fulljournalname: %q", got)
	}
	// An unresolved pmid must be surfaced with its error, never silently dropped.
	bad := recs[1]
	if bad.ID != "999" || bad.Raw["error"] == "" {
		t.Errorf("errored pmid must carry its error field: %+v", bad)
	}
}

func TestPubYear(t *testing.T) {
	for in, want := range map[string]string{
		"2026 Jun": "2026", "1999": "1999", "Jun 2026": "", "": "",
	} {
		if got := pubYear(in); got != want {
			t.Errorf("pubYear(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPubMedRegisteredLiveWithLookup(t *testing.T) {
	src, ok := Get("pubmed")
	if !ok {
		t.Fatal("pubmed must self-register")
	}
	if src.IDField() != "pmid" {
		t.Errorf("IDField=%q want pmid", src.IDField())
	}
	if _, isStaged := src.(stagedSource); isStaged {
		t.Error("pubmed is still the staged placeholder")
	}
	if _, ok := src.(PMIDLookup); !ok {
		t.Error("pubmed source must implement PMIDLookup")
	}
}
