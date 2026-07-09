package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// synthesisFixture populates enough feeds for most probes to read. The
// window-based probes (volume-shift, lifecycle phase) see empty windows and
// read Unknown — which the dossier must exclude from the index.
func synthesisFixture() mockData {
	return mockData{
		eventTypes:    map[string]int{"Death": 10, "Injury": 30, "Malfunction": 60},
		recalls:       5,
		p95:           300,
		sample:        100,
		recallClasses: map[string]int{"Class I": 1, "Class II": 4},
		recallActions: []ComplianceAction{act("20260331", "Class I", "Z-1")},
		trials:        10,
		pubs:          100,
		globalTypes:   map[string]int{"Death": 5, "Injury": 10, "Malfunction": 85},
		reporterTypes: map[string]int{"Health Professional": 40, "COMPANY REPRESENTATIVE": 60},
		missingTotals: map[string]int{"date_of_event": 20},
	}
}

func TestSynthesizeFullDossier(t *testing.T) {
	pinClock(t)
	a := NewSynthesisAnalyzer(synthesisFixture())
	d, err := a.Synthesize(context.Background(), "pacemaker")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Signals) != 11 {
		t.Errorf("signals=%d want 11 (every probe reports)", len(d.Signals))
	}
	if len(d.DataQuality) != 2 {
		t.Errorf("data-quality entries=%d want 2", len(d.DataQuality))
	}
	if d.SignalsMeasured < 5 {
		t.Errorf("measured=%d want >=5 readable signals", d.SignalsMeasured)
	}
	if d.AttentionIndex <= 0 || d.AttentionIndex > 1 {
		t.Errorf("index=%v out of (0,1]", d.AttentionIndex)
	}
	if len(d.Highlights) == 0 || len(d.Highlights) > 3 {
		t.Errorf("highlights=%d want 1..3", len(d.Highlights))
	}
	// Highlights must be sorted by value descending.
	var prev float64 = 2
	for _, h := range d.Highlights {
		var v float64
		if _, err := fmt.Sscanf(h[strings.Index(h, "= ")+2:], "%f", &v); err != nil {
			t.Fatalf("cannot parse highlight value: %q", h)
		}
		if v > prev {
			t.Errorf("highlights not sorted: %v", d.Highlights)
		}
		prev = v
	}
	if !strings.Contains(d.IndexFormula, "NOT risk") {
		t.Error("index formula must disclaim risk")
	}
	if len(d.Notes) != 0 {
		t.Errorf("full fixture should produce no failure notes: %v", d.Notes)
	}
}

// flakyData fails one feed to prove the dossier degrades to partial instead
// of failing or fabricating.
type flakyData struct{ mockData }

func (f flakyData) RecallClassCounts(context.Context, string) (map[string]int, error) {
	return nil, fmt.Errorf("enforcement feed down")
}

func TestSynthesizePartialOnProbeFailure(t *testing.T) {
	pinClock(t)
	a := NewSynthesisAnalyzer(flakyData{synthesisFixture()})
	d, err := a.Synthesize(context.Background(), "pacemaker")
	if err != nil {
		t.Fatalf("one failing probe must not fail the dossier: %v", err)
	}
	if len(d.Notes) != 1 || !strings.Contains(d.Notes[0], "recall-severity") {
		t.Errorf("failing probe must be noted: %v", d.Notes)
	}
	if !strings.Contains(d.Reasoning, "PARTIAL") {
		t.Errorf("partial dossier must say so: %s", d.Reasoning)
	}
	if len(d.Signals) != 10 {
		t.Errorf("signals=%d want 10 (11 probes minus the failed one)", len(d.Signals))
	}
}

func TestSynthesizeNoDataInsufficient(t *testing.T) {
	pinClock(t)
	a := NewSynthesisAnalyzer(mockData{})
	d, err := a.Synthesize(context.Background(), "zzznope")
	if err != nil {
		t.Fatal(err)
	}
	if d.AttentionIndex != 0 || d.SignalsMeasured != 0 {
		t.Errorf("no data: index=%v measured=%d want 0/0", d.AttentionIndex, d.SignalsMeasured)
	}
	if !strings.Contains(d.Reasoning, "insufficient data") ||
		!strings.Contains(d.Reasoning, "not evidence of safety") {
		t.Errorf("reasoning: %s", d.Reasoning)
	}
}

// TestLiveSynthesis grounds the capstone against the real APIs (MDI_LIVE=1).
func TestLiveSynthesis(t *testing.T) {
	if os.Getenv("MDI_LIVE") == "" {
		t.Skip("set MDI_LIVE=1 to run live grounding")
	}
	a := NewSynthesisAnalyzer(NewLiveData())
	d, err := a.Synthesize(context.Background(), "pacemaker")
	if err != nil {
		t.Fatal(err)
	}
	if d.AttentionIndex < 0 || d.AttentionIndex > 1 {
		t.Errorf("index %v out of [0,1]", d.AttentionIndex)
	}
	if d.SignalsMeasured < 5 {
		t.Errorf("pacemaker should yield >=5 readable signals, got %d", d.SignalsMeasured)
	}
	b, _ := json.MarshalIndent(struct {
		Device          string
		AttentionIndex  float64
		SignalsMeasured int
		Highlights      []string
		DataQuality     []string
		Notes           []string
		Reasoning       string
	}{d.Device, d.AttentionIndex, d.SignalsMeasured, d.Highlights, d.DataQuality, d.Notes, d.Reasoning}, "", "  ")
	t.Logf("dossier:\n%s", b)
}
