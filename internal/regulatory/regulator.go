// Package regulatory is the multi-agency adapter layer. Each regulator
// implements Regulator and normalizes its data into model.RegulatoryAction, so
// one table, one query path, and one render path serve every agency. Adding a
// regulator is one new file + one registry line; no existing adapter changes.
//
// Import direction: regulatory may import sources (the FDA adapter reuses the
// openFDA source), but sources must NOT import regulatory.
package regulatory

import (
	"context"
	"sort"

	"github.com/laci141/medical-device-intelligence/internal/model"
)

// DeviceQuery selects regulatory actions for a device/subject.
type DeviceQuery struct {
	Term         string
	Manufacturer string
	Brand        string
	Limit        int
}

// Regulator is the common interface every agency adapter implements.
type Regulator interface {
	Agency() string       // "FDA"
	Jurisdiction() string // "US"
	// Available reports whether a keyless data source is wired. Skeleton
	// agencies return false so an "--agency all" view lists them as pending
	// instead of silently omitting them.
	Available() bool
	Actions(ctx context.Context, q DeviceQuery) ([]model.RegulatoryAction, error)
	Health(ctx context.Context) error
}

var registry = map[string]Regulator{}

// Register adds an agency adapter; called from each adapter's init().
func Register(r Regulator) { registry[r.Agency()] = r }

// Get returns one agency adapter by name (case-sensitive, e.g. "FDA").
func Get(agency string) (Regulator, bool) { r, ok := registry[agency]; return r, ok }

// Agencies returns all registered agency names, sorted for stable output.
func Agencies() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All returns every registered adapter, sorted by agency name.
func All() []Regulator {
	out := make([]Regulator, 0, len(registry))
	for _, n := range Agencies() {
		out = append(out, registry[n])
	}
	return out
}
