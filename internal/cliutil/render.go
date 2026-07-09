package cliutil

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Flags is the output-mode selection shared by every command. The zero value
// means "human plain text".
type Flags struct {
	JSON  bool // --json  : machine JSON envelope
	Agent bool // --agent : machine JSON envelope, non-interactive
	CSV   bool // --csv   : CSV rows on stdout, disclaimer on stderr
	Plain bool // --plain : force human plain text
}

// machine reports whether the caller explicitly asked for a machine envelope.
//
// This is the fix for the guardrail-8 bug: the machine path is taken ONLY when
// --json or --agent is set. It is NEVER inferred from stdout being a pipe. A
// piped plain-text consumer therefore still gets the disclaimer and legend.
func (f Flags) machine() bool { return f.JSON || f.Agent }

// KV is one ordered headline key/value shown above the records. Ordered (a
// slice, not a map) so composite views render deterministically.
type KV struct {
	Key   string
	Value any
}

// Meta carries the safety text and optional headline attached to a result.
type Meta struct {
	// Legend is an optional explanatory legend (e.g. the FDA class legend).
	Legend string
	// EmptyMsg, when the result set is empty, is shown instead of rows. It must
	// read as "no records found" and never imply the subject is safe.
	EmptyMsg string
	// Summary is an optional ordered headline (e.g. severity counts for a
	// composite view) rendered before the records in every mode.
	Summary []KV
}

// Output renders records in the mode selected by flags. records is a slice of
// ordered field maps (one map per row). Disclaimer handling:
//
//   - --json/--agent : envelope {records, disclaimer, [legend]} on stdout.
//   - --csv          : CSV rows on stdout; disclaimer (+legend) on stderr so the
//     CSV stays machine-clean (guardrail 9).
//   - plain/piped    : rows + legend + disclaimer footer on stdout (guardrail 8).
func Output(stdout, stderr io.Writer, records []map[string]any, meta Meta, f Flags) error {
	if f.machine() {
		return outputJSON(stdout, records, meta)
	}
	if f.CSV {
		return outputCSV(stdout, stderr, records, meta)
	}
	return outputPlain(stdout, records, meta)
}

// OutputValue writes an arbitrary value as an indented JSON object with the
// shared disclaimer folded in. Used by commands (e.g. dossier) whose --json
// payload is a full struct rather than a rows-and-meta envelope.
func OutputValue(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	obj["disclaimer"] = Disclaimer
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(obj)
}

func outputJSON(w io.Writer, records []map[string]any, meta Meta) error {
	env := map[string]any{
		"records":    records,
		"count":      len(records),
		"disclaimer": Disclaimer,
	}
	if len(meta.Summary) > 0 {
		sum := make(map[string]any, len(meta.Summary))
		for _, kv := range meta.Summary {
			sum[kv.Key] = kv.Value
		}
		env["summary"] = sum
	}
	if meta.Legend != "" {
		env["legend"] = meta.Legend
	}
	if len(records) == 0 && meta.EmptyMsg != "" {
		env["message"] = meta.EmptyMsg
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

func outputCSV(stdout, stderr io.Writer, records []map[string]any, meta Meta) error {
	// Headline goes to stderr so the CSV on stdout stays machine-clean.
	writeSummary(stderr, meta.Summary)
	if len(records) == 0 {
		// Disclaimer still goes to stderr; stdout stays empty for clean piping.
		if meta.EmptyMsg != "" {
			fmt.Fprintln(stderr, meta.EmptyMsg)
		}
		fmt.Fprintln(stderr, Disclaimer)
		return nil
	}
	cols := columns(records)
	cw := csv.NewWriter(stdout)
	if err := cw.Write(cols); err != nil {
		return err
	}
	for _, r := range records {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = fmt.Sprintf("%v", valueOrEmpty(r, c))
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	// Safety text to stderr, keeping CSV rows on stdout machine-clean.
	if meta.Legend != "" {
		fmt.Fprintln(stderr, meta.Legend)
	}
	fmt.Fprintln(stderr, Disclaimer)
	return nil
}

func outputPlain(w io.Writer, records []map[string]any, meta Meta) error {
	writeSummary(w, meta.Summary)
	if len(records) == 0 {
		msg := meta.EmptyMsg
		if msg == "" {
			msg = NoRecordsMsg
		}
		fmt.Fprintln(w, msg)
		fmt.Fprintln(w)
		fmt.Fprintln(w, Disclaimer)
		return nil
	}
	cols := columns(records)
	for i, r := range records {
		if i > 0 {
			fmt.Fprintln(w)
		}
		for _, c := range cols {
			fmt.Fprintf(w, "  %-24s %v\n", c+":", valueOrEmpty(r, c))
		}
	}
	fmt.Fprintln(w)
	if meta.Legend != "" {
		fmt.Fprintln(w, meta.Legend)
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, Disclaimer)
	return nil
}

// writeSummary prints the ordered headline block, if any, followed by a blank
// line. Used by the plain (stdout) and CSV (stderr) paths.
func writeSummary(w io.Writer, summary []KV) {
	if len(summary) == 0 {
		return
	}
	for _, kv := range summary {
		fmt.Fprintf(w, "%-24s %v\n", kv.Key+":", kv.Value)
	}
	fmt.Fprintln(w)
}

// columns returns a stable, sorted union of keys across records.
func columns(records []map[string]any) []string {
	seen := map[string]bool{}
	for _, r := range records {
		for k := range r {
			seen[k] = true
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

func valueOrEmpty(r map[string]any, k string) any {
	if v, ok := r[k]; ok {
		return v
	}
	return ""
}
