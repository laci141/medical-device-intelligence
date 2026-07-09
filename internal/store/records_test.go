package store

import "testing"

func rec(source, id, term string) Record {
	return Record{Source: source, ID: id, Term: term, Date: "20240101",
		Summary: "s", Raw: map[string]any{"k": "v"}}
}

func TestUpsertRecordsIdempotentNewCount(t *testing.T) {
	s := openTemp(t)
	n, err := s.UpsertRecords([]Record{
		rec("openfda_device_enforcement", "Z-1", "pacemaker"),
		rec("pubmed", "Z-1", "pacemaker"), // same id, different source: distinct row
		rec("pubmed", "P-2", "pacemaker"),
	}, 2) // batch smaller than the slice → exercises chunked transactions
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("new=%d want 3", n)
	}
	// Re-sync: same rows again plus one genuinely new one → new must be 1.
	n, err = s.UpsertRecords([]Record{
		rec("pubmed", "P-2", "pacemaker"),
		rec("pubmed", "P-3", "pacemaker"),
	}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("re-sync new=%d want 1 (updates are not new)", n)
	}
	total, _ := s.CountRecords()
	if total != 4 {
		t.Fatalf("total=%d want 4", total)
	}
}

func TestUpsertRecordsRejectsEmptyID(t *testing.T) {
	s := openTemp(t)
	if _, err := s.UpsertRecords([]Record{rec("src", "", "t")}, 10); err != ErrEmptyID {
		t.Fatalf("empty id: got %v want ErrEmptyID", err)
	}
	if _, err := s.UpsertRecords([]Record{rec("", "id", "t")}, 10); err != ErrEmptyID {
		t.Fatalf("empty source: got %v want ErrEmptyID", err)
	}
	if n, _ := s.CountRecords(); n != 0 {
		t.Fatalf("nothing should be stored, got %d", n)
	}
}

func TestAllRecordsDeterministicOrder(t *testing.T) {
	s := openTemp(t)
	if _, err := s.UpsertRecords([]Record{
		rec("pubmed", "B", "x"), rec("clinicaltrials", "A", "x"), rec("pubmed", "A", "x"),
	}, 100); err != nil {
		t.Fatal(err)
	}
	rows, err := s.AllRecords()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Source+"/"+r.RecordID)
	}
	want := []string{"clinicaltrials/A", "pubmed/A", "pubmed/B"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order got %v want %v", got, want)
		}
	}
}

func TestSyncRunBookkeeping(t *testing.T) {
	s := openTemp(t)
	if _, ok, err := s.LastSyncTime("pacemaker"); err != nil || ok {
		t.Fatalf("fresh db: ok=%v err=%v want none", ok, err)
	}
	if err := s.RecordSyncRun("pacemaker", 3, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSyncRun("pacemaker", 1, 4); err != nil {
		t.Fatal(err)
	}
	ts, ok, err := s.LastSyncTime("pacemaker")
	if err != nil || !ok || ts == "" {
		t.Fatalf("want newest run time, got ts=%q ok=%v err=%v", ts, ok, err)
	}
	if _, ok, _ := s.LastSyncTime("otherterm"); ok {
		t.Fatal("terms must not share sync history")
	}
}
