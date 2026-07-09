package intelligence

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func act(date, class, ref string) ComplianceAction {
	return ComplianceAction{Date: date, Type: "recall (" + class + ")",
		Description: "device model X", Reference: ref}
}

func TestFDAStatusOK(t *testing.T) {
	a := NewComplianceAnalyzer(mockData{}) // no recalls at all
	st, err := a.CheckFDAStatus(context.Background(), "cleandevice")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusOK || st.Severity != 0 || len(st.Actions) != 0 {
		t.Errorf("got %+v want OK/0/no actions", st)
	}
	if !strings.Contains(st.Reasoning, "not evidence of safety") {
		t.Error("OK must never read as a clean bill")
	}
	if st.ConfidenceLevel != ConfidenceLow {
		t.Errorf("confidence=%q want LOW on empty record", st.ConfidenceLevel)
	}
}

func TestFDAStatusWarningClassIIIOnly(t *testing.T) {
	a := NewComplianceAnalyzer(mockData{
		recallClasses: map[string]int{"Class III": 2},
		recallActions: []ComplianceAction{act("20230105", "Class III", "Z-1")},
	})
	st, err := a.CheckFDAStatus(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusWarning {
		t.Errorf("status=%q want WARNING", st.Status)
	}
	if st.Severity != 0.2 { // all Class III
		t.Errorf("severity=%v want 0.2", st.Severity)
	}
}

func TestFDAStatusRecallClassII(t *testing.T) {
	a := NewComplianceAnalyzer(mockData{
		recallClasses: map[string]int{"Class II": 4},
		recallActions: []ComplianceAction{
			act("20240110", "Class II", "Z-1"), act("20230601", "Class II", "Z-2"),
		},
	})
	st, err := a.CheckFDAStatus(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusRecall {
		t.Errorf("status=%q want RECALL", st.Status)
	}
	if st.Severity != 0.5 {
		t.Errorf("severity=%v want 0.5", st.Severity)
	}
	if st.LastAction != "20240110" {
		t.Errorf("last action=%q want 20240110", st.LastAction)
	}
	if len(st.Actions) != 2 || !strings.Contains(st.Actions[0], "Z-1") {
		t.Errorf("actions must cite recall numbers: %v", st.Actions)
	}
}

func TestFDAStatusCriticalMultipleClassI(t *testing.T) {
	a := NewComplianceAnalyzer(mockData{
		recallClasses: map[string]int{"Class I": 3, "Class II": 10},
		recallActions: []ComplianceAction{act("20250301", "Class I", "Z-9")},
	})
	st, err := a.CheckFDAStatus(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusCritical {
		t.Errorf("status=%q want CRITICAL (3 Class I)", st.Status)
	}
	// (3*1.0 + 10*0.5)/13 ≈ 0.62
	if st.Severity != 0.62 {
		t.Errorf("severity=%v want 0.62", st.Severity)
	}
	for _, want := range []string{"CRITICAL", "3 Class I", "warning letters/approvals not wired"} {
		if !strings.Contains(st.Reasoning, want) {
			t.Errorf("reasoning missing %q: %s", want, st.Reasoning)
		}
	}
}

func TestRegulatoryTimelineChronological(t *testing.T) {
	a := NewComplianceAnalyzer(mockData{recallActions: []ComplianceAction{
		act("20240110", "Class II", "Z-NEW"),
		act("", "Class II", "Z-UNDATED"),
		act("20190117", "Class I", "Z-OLD"),
	}})
	tl, err := a.GetRegulatoryTimeline(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(tl) != 3 {
		t.Fatalf("len=%d want 3", len(tl))
	}
	if tl[0].Reference != "Z-OLD" || tl[1].Reference != "Z-NEW" || tl[2].Reference != "Z-UNDATED" {
		t.Errorf("order wrong (want oldest first, undated last): %v", tl)
	}
}

// TestLiveCompliance grounds Module 04 against the real API (MDI_LIVE=1).
func TestLiveCompliance(t *testing.T) {
	if os.Getenv("MDI_LIVE") == "" {
		t.Skip("set MDI_LIVE=1 to run live grounding")
	}
	a := NewComplianceAnalyzer(NewLiveData())
	ctx := context.Background()

	st, err := a.CheckFDAStatus(ctx, "pacemaker")
	if err != nil {
		t.Fatal(err)
	}
	if st.Severity < 0 || st.Severity > 1 {
		t.Errorf("severity %v out of [0,1]", st.Severity)
	}
	b, _ := json.MarshalIndent(struct {
		Status, LastAction, Reasoning string
		Severity                      float64
		ActionCount                   int
	}{st.Status, st.LastAction, st.Reasoning, st.Severity, len(st.Actions)}, "", "  ")
	t.Logf("fda-status:\n%s", b)

	tl, err := a.GetRegulatoryTimeline(ctx, "pacemaker")
	if err != nil {
		t.Fatal(err)
	}
	if len(tl) == 0 {
		t.Fatal("pacemaker timeline should not be empty")
	}
	for i := 1; i < len(tl); i++ {
		if tl[i-1].Date != "" && tl[i].Date != "" && tl[i-1].Date > tl[i].Date {
			t.Errorf("timeline not chronological at %d: %s > %s", i, tl[i-1].Date, tl[i].Date)
		}
	}
	t.Logf("timeline: %d actions, first %s [%s], last %s [%s]",
		len(tl), tl[0].Date, tl[0].Reference, tl[len(tl)-1].Date, tl[len(tl)-1].Reference)
}
