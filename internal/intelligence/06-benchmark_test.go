package intelligence

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEventRankPercentile(t *testing.T) {
	a := NewBenchmarkAnalyzer(mockData{
		eventTypes: map[string]int{"Malfunction": 500},
		typeVolumes: map[string]int{
			"A": 100, "B": 200, "C": 400, "D": 800, "E": 1600,
		},
	})
	sig, err := a.AnalyzeEventRank(context.Background(), "pacemaker")
	if err != nil {
		t.Fatal(err)
	}
	// 500 sits above 3 of 5 peers → 0.6 High.
	if sig.Value != 0.6 || sig.Label != LabelHigh {
		t.Errorf("got %v/%q want 0.6/High", sig.Value, sig.Label)
	}
	for _, want := range []string{"above 3 of the 5", "not risk"} {
		if !strings.Contains(sig.Reasoning, want) {
			t.Errorf("reasoning missing %q: %s", want, sig.Reasoning)
		}
	}
}

func TestSeverityDeltaAboveGlobal(t *testing.T) {
	a := NewBenchmarkAnalyzer(mockData{
		// device severity: all deaths among 100 → 1.0
		eventTypes: map[string]int{"Death": 100},
		// global severity: all malfunction → 0.3 ... delta 0.7 → saturates 1.0
		globalTypes: map[string]int{"Malfunction": 100000},
	})
	sig, err := a.AnalyzeSeverityDelta(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if sig.Value != 1.0 || sig.Label != LabelCritical {
		t.Errorf("got %v/%q want 1.0/Critical", sig.Value, sig.Label)
	}
	if !strings.Contains(sig.Reasoning, "above the global") {
		t.Errorf("reasoning: %s", sig.Reasoning)
	}

	// Below-global mix must read 0/Low, direction stated.
	a = NewBenchmarkAnalyzer(mockData{
		eventTypes:  map[string]int{"Malfunction": 100},
		globalTypes: map[string]int{"Death": 50, "Malfunction": 50},
	})
	sig, err = a.AnalyzeSeverityDelta(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if sig.Value != 0 || !strings.Contains(sig.Reasoning, "below") {
		t.Errorf("below-global: got %v %q", sig.Value, sig.Reasoning)
	}
}

func TestRecallRateVsGlobal(t *testing.T) {
	a := NewBenchmarkAnalyzer(mockData{
		eventTypes: map[string]int{"Malfunction": 1000}, // device: 1000 events
		recalls:    4,                                   // 4 per 1000
		globalTypes: map[string]int{
			"Malfunction": 1000000, // global: 1e6 events
		},
		globalClasses: map[string]int{"Class II": 2000}, // 2 per 1000 global
	})
	sig, err := a.AnalyzeRecallRate(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	// ratio 2x → value 2/4 = 0.5 Medium.
	if sig.Value != 0.5 || sig.Label != LabelMedium {
		t.Errorf("got %v/%q want 0.5/Medium", sig.Value, sig.Label)
	}
	for _, want := range []string{"4.00 recalls per 1000", "2.0x", "vigilant recall practice"} {
		if !strings.Contains(sig.Reasoning, want) {
			t.Errorf("reasoning missing %q: %s", want, sig.Reasoning)
		}
	}
}

func TestBenchmarkNoDataIsGracefulUnknown(t *testing.T) {
	a := NewBenchmarkAnalyzer(mockData{})
	ctx := context.Background()
	for name, f := range map[string]func() (*Signal, error){
		"event-rank":     func() (*Signal, error) { return a.AnalyzeEventRank(ctx, "zzz") },
		"severity-delta": func() (*Signal, error) { return a.AnalyzeSeverityDelta(ctx, "zzz") },
		"recall-rate":    func() (*Signal, error) { return a.AnalyzeRecallRate(ctx, "zzz") },
	} {
		sig, err := f()
		if err != nil {
			t.Fatalf("%s: no data must not error: %v", name, err)
		}
		if sig.Label != LabelUnknown || sig.Value != 0 {
			t.Errorf("%s no-data: %+v want Unknown/0", name, sig)
		}
	}
}

// TestLiveBenchmark grounds Module 06 against the real APIs (MDI_LIVE=1).
func TestLiveBenchmark(t *testing.T) {
	if os.Getenv("MDI_LIVE") == "" {
		t.Skip("set MDI_LIVE=1 to run live grounding")
	}
	a := NewBenchmarkAnalyzer(NewLiveData())
	ctx := context.Background()
	for name, f := range map[string]func() (*Signal, error){
		"event-rank":     func() (*Signal, error) { return a.AnalyzeEventRank(ctx, "pacemaker") },
		"severity-delta": func() (*Signal, error) { return a.AnalyzeSeverityDelta(ctx, "pacemaker") },
		"recall-rate":    func() (*Signal, error) { return a.AnalyzeRecallRate(ctx, "pacemaker") },
	} {
		sig, err := f()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sig.Value < 0 || sig.Value > 1 {
			t.Errorf("%s: value %v out of [0,1]", name, sig.Value)
		}
		b, _ := json.MarshalIndent(sig, "", "  ")
		t.Logf("%s:\n%s", name, b)
	}
}
