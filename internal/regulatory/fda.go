package regulatory

import (
	"context"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/model"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

// FDA is the LIVE regulator adapter. It reuses the openFDA device enforcement
// source rather than fetching independently, and normalizes recalls into
// model.RegulatoryAction (action_type "recall").
type FDA struct {
	src sources.Source
}

// NewFDA constructs the FDA adapter over the registered openFDA source.
func NewFDA() *FDA {
	s, _ := sources.Get("openfda_device_enforcement")
	return &FDA{src: s}
}

func (f *FDA) Agency() string       { return "FDA" }
func (f *FDA) Jurisdiction() string { return "US" }
func (f *FDA) Available() bool      { return f.src != nil }

func (f *FDA) Actions(ctx context.Context, q DeviceQuery) ([]model.RegulatoryAction, error) {
	if f.src == nil {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 25
	}
	recs, _, err := f.src.Fetch(ctx, sources.Query{Term: q.Term, Limit: limit})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	actions := make([]model.RegulatoryAction, 0, len(recs))
	for _, r := range recs {
		actions = append(actions, model.RegulatoryAction{
			Agency:       "FDA",
			Jurisdiction: "US",
			ActionType:   "recall",
			Status:       str(r.Raw["status"]),
			Date:         str(r.Raw["recall_initiation_date"]),
			Reference:    r.ID,
			Source: model.SourceRef{
				Source:    f.src.Name(),
				RecordID:  r.ID,
				FetchedAt: now,
			},
		})
	}
	return actions, nil
}

func (f *FDA) Health(ctx context.Context) error {
	if f.src == nil {
		return nil
	}
	return f.src.Health(ctx)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func init() { Register(NewFDA()) }
