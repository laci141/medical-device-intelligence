package regulatory

import (
	"context"

	"github.com/laci141/medical-device-intelligence/internal/model"
)

// The adapters below are compile-ready skeletons. There is no keyless machine
// API for their device data today (EU EUDAMED is a partial rollout; PMDA has
// minimal English machine access), so Available() is false and Actions()
// returns no data with no error. When a keyless source appears, wiring one is a
// single-file change — the registry, store table, query path, and render path
// are already shared. This is the "new regulator = new file, old code
// untouched" property in practice.
type skeleton struct {
	agency       string
	jurisdiction string
}

func (s skeleton) Agency() string       { return s.agency }
func (s skeleton) Jurisdiction() string { return s.jurisdiction }
func (s skeleton) Available() bool      { return false }
func (s skeleton) Actions(context.Context, DeviceQuery) ([]model.RegulatoryAction, error) {
	return nil, nil
}
func (s skeleton) Health(context.Context) error { return nil }

func init() {
	Register(skeleton{agency: "EMA", jurisdiction: "EU"})
	Register(skeleton{agency: "HealthCanada", jurisdiction: "CA"})
	Register(skeleton{agency: "TGA", jurisdiction: "AU"})
	Register(skeleton{agency: "PMDA", jurisdiction: "JP"})
}
