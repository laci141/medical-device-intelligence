package sources

import "context"

// The adapters below are compile-ready and self-register, but their live Fetch
// is staged for a later phase: they return ErrNotWired rather than a silent
// empty result, so a caller can report "source not yet integrated" honestly.
// Wiring one live means implementing Fetch/Health in its own file — no other
// adapter changes. This is the plugin pattern the architecture promised.

type stagedSource struct {
	name    string
	idField string
}

func (s stagedSource) Name() string    { return s.name }
func (s stagedSource) IDField() string { return s.idField }
func (s stagedSource) Fetch(context.Context, Query) ([]RawRecord, Page, error) {
	return nil, Page{}, ErrNotWired
}
func (s stagedSource) Health(context.Context) error { return ErrNotWired }

func init() {
	// clinicaltrials and pubmed went live in Phase 2b Group 3; they must NOT be
	// re-registered here or this init (running after theirs) would clobber the
	// live adapters in the registry.
	Register(stagedSource{name: "openalex", idField: "id"})
	Register(stagedSource{name: "crossref", idField: "DOI"})
}
