package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// getJSON fetches a URL and decodes the JSON body, failing the test on any
// transport or decode error.
func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("GET %s: Content-Type=%q want application/json", url, ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: read body: %v", url, err)
	}
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("GET %s: body is not JSON: %v\n%s", url, err, body)
	}
	return resp.StatusCode, v
}

func TestServeHealth(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/health")
	if status != http.StatusOK {
		t.Fatalf("health status=%d want 200", status)
	}
	if v["status"] != "ok" {
		t.Errorf("health status field=%v want ok", v["status"])
	}
	if v["commands"] != float64(len(commands)) {
		t.Errorf("health commands=%v want %d (the live registry size)", v["commands"], len(commands))
	}
	if v["modules"] != float64(moduleCount) {
		t.Errorf("health modules=%v want %d", v["modules"], moduleCount)
	}
	if _, ok := v["disclaimer"]; !ok {
		t.Error("health must carry the disclaimer")
	}
}

func TestServeSignalsWithDevice(t *testing.T) {
	withDossier(t, sampleDossier(), nil)
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/signals?device=pacemaker")
	if status != http.StatusOK {
		t.Fatalf("signals status=%d want 200: %v", status, v)
	}
	if _, ok := v["records"]; !ok {
		t.Error("signals must return the --json records envelope")
	}
	if _, ok := v["disclaimer"]; !ok {
		t.Error("signals must carry the disclaimer")
	}
	sum, _ := v["summary"].(map[string]any)
	if sum["device"] != "pacemaker" {
		t.Errorf("signals summary device=%v want pacemaker", sum["device"])
	}
}

func TestServeSignalsMissingDevice400(t *testing.T) {
	withDossier(t, sampleDossier(), nil)
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/signals")
	if status != http.StatusBadRequest {
		t.Fatalf("signals without device status=%d want 400", status)
	}
	if v["error"] != "device required" {
		t.Errorf("error=%v want %q", v["error"], "device required")
	}
}

func TestServeDossierWithDevice(t *testing.T) {
	withDossier(t, sampleDossier(), nil)
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/dossier?device=pacemaker")
	if status != http.StatusOK {
		t.Fatalf("dossier status=%d want 200: %v", status, v)
	}
	if v["attention_index"] != 0.47 {
		t.Errorf("attention_index=%v want 0.47", v["attention_index"])
	}
}

func TestServeCompareMissingParam400(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/compare?a=pacemaker")
	if status != http.StatusBadRequest {
		t.Fatalf("compare missing b status=%d want 400", status)
	}
	if v["error"] != "b required" {
		t.Errorf("error=%v want %q", v["error"], "b required")
	}
}

func TestServeUnknownRoute404(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/nonexistent")
	if status != http.StatusNotFound {
		t.Fatalf("unknown route status=%d want 404", status)
	}
	if v["error"] != "not found" {
		t.Errorf("error=%v want %q", v["error"], "not found")
	}
}

func TestServeNonGET405(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST health status=%d want 405", resp.StatusCode)
	}
}

func TestServeCORSHeader(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin=%q want *", got)
	}
}

func TestServeSynthesizeErrorIsJSONNotPanic(t *testing.T) {
	withDossier(t, nil, io.ErrUnexpectedEOF)
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/signals?device=pacemaker")
	if status != http.StatusBadGateway {
		t.Fatalf("signals with failing module status=%d want 502", status)
	}
	errMsg, _ := v["error"].(string)
	if errMsg == "" {
		t.Error("module failure must surface as a JSON error field")
	}
}

func TestServeRootServesFrontend(t *testing.T) {
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status=%d want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type=%q want text/html", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET /: read body: %v", err)
	}
	for _, want := range []string{"<!DOCTYPE html>", "Devicera", "/api/signals"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("frontend HTML missing %q", want)
		}
	}
}

func TestServeSignalsJSONFullReasoning(t *testing.T) {
	d := sampleDossier()
	long := strings.Repeat("x", 200) // well past the 90-char plain-mode clip
	d.Signals[0].Reasoning = long
	withDossier(t, d, nil)
	ts := httptest.NewServer(NewServeHandler())
	defer ts.Close()

	status, v := getJSON(t, ts.URL+"/api/signals?device=pacemaker")
	if status != http.StatusOK {
		t.Fatalf("signals status=%d want 200", status)
	}
	recs, _ := v["records"].([]any)
	if len(recs) == 0 {
		t.Fatal("signals returned no records")
	}
	first, _ := recs[0].(map[string]any)
	got, _ := first["reasoning"].(string)
	if got != long {
		t.Errorf("JSON reasoning clipped: len=%d want %d (must be the full text)", len(got), len(long))
	}
}

func TestServeCommandRegistered(t *testing.T) {
	if _, ok := commands["serve"]; !ok {
		t.Error("command \"serve\" not registered")
	}
}

// deviceRow must translate every device_class code — a raw one-character code
// ("U", "N", "1", …) reaching the table or the exports is a bug.
func TestDeviceRowClassNeverRaw(t *testing.T) {
	rawFor := func(class any) map[string]any {
		pc := map[string]any{"openfda": map[string]any{"device_class": class}}
		return map[string]any{"product_codes": []any{pc}}
	}
	cases := map[string]string{
		"1": "Class I",
		"2": "Class II",
		"3": "Class III",
		"U": "Unclassified",
		"N": "Unclassified",
		"f": "Unclassified",
		"":  "Not specified",
	}
	for in, want := range cases {
		got, _ := deviceRow(rawFor(in))["device_class"].(string)
		if got != want {
			t.Errorf("device_class %q => %q, want %q", in, got, want)
		}
		if len(got) <= 1 {
			t.Errorf("device_class %q leaked as raw code %q", in, got)
		}
	}
	// No product_codes at all must still yield a translated value.
	if got, _ := deviceRow(map[string]any{})["device_class"].(string); got != "Not specified" {
		t.Errorf("missing product_codes => %q, want \"Not specified\"", got)
	}
}

// openFDA's UDI endpoint serializes booleans as the strings "true"/"false";
// sterilization and latex labeling must be read from either representation.
func TestDeviceRowStringBooleans(t *testing.T) {
	raw := map[string]any{
		"sterilization":        map[string]any{"is_sterile": "true"},
		"is_labeled_as_no_nrl": "true",
	}
	row := deviceRow(raw)
	if row["sterilization"] != "Sterile" {
		t.Errorf("string is_sterile=true => %q, want \"Sterile\"", row["sterilization"])
	}
	if row["latex"] != "Labeled latex-free" {
		t.Errorf("string is_labeled_as_no_nrl=true => %q, want \"Labeled latex-free\"", row["latex"])
	}

	raw = map[string]any{
		"sterilization":        map[string]any{"is_sterile": false},
		"is_labeled_as_no_nrl": "false",
	}
	row = deviceRow(raw)
	if row["sterilization"] != "Non-sterile" {
		t.Errorf("bool is_sterile=false => %q, want \"Non-sterile\"", row["sterilization"])
	}
	if row["latex"] != "Not labeled latex-free" {
		t.Errorf("string is_labeled_as_no_nrl=false => %q, want \"Not labeled latex-free\"", row["latex"])
	}
}
