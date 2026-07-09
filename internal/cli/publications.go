package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
	"github.com/laci141/medical-device-intelligence/internal/sources"
)

func init() { register("publications", cmdPublications) }

// cmdPublications searches PubMed for literature on a device term, or resolves
// specific records with --pmid. Each row cites its PMID (and DOI when PubMed
// carries one). Publication volume is not a quality or safety verdict.
func cmdPublications(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, f := newFlagSet("publications")
	limit := fs.Int("limit", 10, "max publications to list (>=1)")
	pmid := fs.String("pmid", "", "look up specific PMIDs (comma-separated) instead of searching")
	if err := parse(fs, stderr, args, map[string]bool{"limit": true, "pmid": true}); err != nil {
		return 2
	}
	if *pmid == "" && fs.NArg() < 1 {
		fmt.Fprintln(stderr, "publications: a term or --pmid is required, e.g. publications pacemaker")
		return 2
	}
	if *pmid != "" && fs.NArg() > 0 {
		fmt.Fprintln(stderr, "publications: use a term or --pmid, not both")
		return 2
	}
	if *limit < 1 {
		fmt.Fprintln(stderr, "publications: --limit must be >= 1")
		return 2
	}

	src, ok := getSource("pubmed")
	if !ok {
		fmt.Fprintln(stderr, "publications: pubmed source unavailable")
		return 1
	}

	var recs []sources.RawRecord
	summary := []cliutil.KV{}
	if *pmid != "" {
		lookup, ok := src.(sources.PMIDLookup)
		if !ok {
			fmt.Fprintln(stderr, "publications: pubmed source does not support PMID lookup")
			return 1
		}
		ids := splitPMIDs(*pmid)
		if len(ids) == 0 {
			fmt.Fprintln(stderr, "publications: --pmid needs at least one id")
			return 2
		}
		all, err := lookup.LookupPMIDs(ctx, ids)
		if err != nil {
			fmt.Fprintf(stderr, "publications: %v\n", err)
			return 1
		}
		// An unknown PMID resolves to an entry with an error field. Keep the
		// record rows clean (resolved only) but name the failures in the
		// headline — unresolved ids are reported, never silently dropped.
		var unresolved []string
		for _, r := range all {
			if str(r.Raw["error"]) != "" {
				unresolved = append(unresolved, r.ID)
				continue
			}
			recs = append(recs, r)
		}
		summary = append(summary, cliutil.KV{Key: "pmids_requested", Value: len(ids)})
		if len(unresolved) > 0 {
			summary = append(summary, cliutil.KV{
				Key: "pmids_unresolved", Value: joinComma(unresolved),
			})
		}
	} else {
		term := fs.Arg(0)
		var page sources.Page
		var err error
		recs, page, err = src.Fetch(ctx, sources.Query{Term: term, Limit: *limit})
		if err != nil {
			fmt.Fprintf(stderr, "publications: %v\n", err)
			return 1
		}
		summary = append(summary,
			cliutil.KV{Key: "term", Value: term},
			cliutil.KV{Key: "publications_total", Value: page.Total},
		)
	}
	summary = append(summary, cliutil.KV{
		Key: "note", Value: "publication volume is not a quality or safety verdict",
	})

	rows := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := map[string]any{
			"pmid":    r.ID,
			"year":    str(r.Raw["year"]),
			"title":   clip(str(r.Raw["title"]), 140),
			"journal": clip(str(r.Raw["journal"]), 80),
			"doi":     str(r.Raw["doi"]),
		}
		// An unresolved PMID comes back with an error field; surface it honestly.
		if e := str(r.Raw["error"]); e != "" {
			row["error"] = e
		}
		rows = append(rows, row)
	}

	meta := cliutil.Meta{
		Summary:  summary,
		EmptyMsg: "no publications found in PubMed",
	}
	if err := cliutil.Output(stdout, stderr, rows, meta, *f); err != nil {
		fmt.Fprintf(stderr, "publications: %v\n", err)
		return 1
	}
	return 0
}

// splitPMIDs parses a comma-separated PMID list, dropping empties.
func splitPMIDs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
