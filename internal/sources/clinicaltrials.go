package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

// ClinicalTrials is the LIVE ClinicalTrials.gov v2 adapter. Endpoint:
// https://clinicaltrials.gov/api/v2/studies. Keyless.
//
// Query mapping (verified live 2026-07-09):
//   - Term  -> query.intr (intervention search) combined with
//     filter.advanced=AREA[InterventionType]DEVICE so results are device
//     interventions, not drugs that merely mention the term.
//   - Firm  -> query.spons (sponsor search).
//   - Limit -> pageSize; total comes from countTotal=true -> totalCount.
//
// v2 paginates with opaque nextPageToken values, not offsets, so Query.Skip is
// rejected explicitly rather than silently ignored.
type ClinicalTrials struct {
	client *cliutil.Client
}

func NewClinicalTrials() *ClinicalTrials {
	return &ClinicalTrials{client: cliutil.NewClient("https://clinicaltrials.gov")}
}

func (s *ClinicalTrials) Name() string { return "clinicaltrials" }

// IDField: the NCT registry number, flattened into each record's Raw map.
func (s *ClinicalTrials) IDField() string { return "nct_id" }

// ctgovFields trims the response to what we render; full study records are
// large. Verified live: the pipe-separated "special" field names work on v2.
const ctgovFields = "NCTId|BriefTitle|OverallStatus|Phase|Condition|InterventionName"

func (s *ClinicalTrials) params(q Query) (url.Values, error) {
	if q.Term == "" {
		return nil, fmt.Errorf("clinicaltrials search requires a device term")
	}
	if q.Skip > 0 {
		return nil, fmt.Errorf("clinicaltrials does not support skip pagination (v2 uses page tokens)")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 25
	}
	v := url.Values{}
	v.Set("query.intr", q.Term)
	v.Set("filter.advanced", "AREA[InterventionType]DEVICE")
	if q.Firm != "" {
		v.Set("query.spons", q.Firm)
	}
	v.Set("fields", ctgovFields)
	v.Set("pageSize", strconv.Itoa(limit))
	v.Set("countTotal", "true")
	return v, nil
}

// ctgovEnvelope mirrors the slice of the v2 response we consume.
type ctgovEnvelope struct {
	TotalCount int `json:"totalCount"`
	Studies    []struct {
		ProtocolSection struct {
			IdentificationModule struct {
				NCTID      string `json:"nctId"`
				BriefTitle string `json:"briefTitle"`
			} `json:"identificationModule"`
			StatusModule struct {
				OverallStatus string `json:"overallStatus"`
			} `json:"statusModule"`
			ConditionsModule struct {
				Conditions []string `json:"conditions"`
			} `json:"conditionsModule"`
			DesignModule struct {
				Phases []string `json:"phases"`
			} `json:"designModule"`
			ArmsInterventionsModule struct {
				Interventions []struct {
					Name string `json:"name"`
				} `json:"interventions"`
			} `json:"armsInterventionsModule"`
		} `json:"protocolSection"`
	} `json:"studies"`
}

func (s *ClinicalTrials) Fetch(ctx context.Context, q Query) ([]RawRecord, Page, error) {
	params, err := s.params(q)
	if err != nil {
		return nil, Page{}, err
	}
	body, _, err := s.client.GetJSON(ctx, "/api/v2/studies", params)
	if err != nil {
		return nil, Page{}, err
	}
	return parseCTGov(body)
}

// parseCTGov flattens each study into a one-level Raw map keyed by the fields
// the command layer renders; "no matches" is a 200 with an empty studies list.
func parseCTGov(body []byte) ([]RawRecord, Page, error) {
	var env ctgovEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, Page{}, err
	}
	recs := make([]RawRecord, 0, len(env.Studies))
	for _, st := range env.Studies {
		p := st.ProtocolSection
		names := make([]string, 0, len(p.ArmsInterventionsModule.Interventions))
		for _, iv := range p.ArmsInterventionsModule.Interventions {
			if iv.Name != "" {
				names = append(names, iv.Name)
			}
		}
		recs = append(recs, RawRecord{
			ID: p.IdentificationModule.NCTID,
			Raw: map[string]any{
				"nct_id":        p.IdentificationModule.NCTID,
				"title":         p.IdentificationModule.BriefTitle,
				"status":        p.StatusModule.OverallStatus,
				"phase":         strings.Join(p.DesignModule.Phases, ", "),
				"conditions":    strings.Join(p.ConditionsModule.Conditions, ", "),
				"interventions": strings.Join(names, ", "),
			},
		})
	}
	return recs, Page{Total: env.TotalCount, Returned: len(recs)}, nil
}

// TrialStatusTotal returns the device-intervention study total restricted to
// the given overall statuses (filter.overallStatus pipe-list, verified live
// 2026-07-09: RECRUITING|ACTIVE_NOT_RECRUITING → 88 for pacemaker).
func (s *ClinicalTrials) TrialStatusTotal(ctx context.Context, device string, statuses []string) (int, error) {
	if device == "" || len(statuses) == 0 {
		return 0, fmt.Errorf("trial status total requires a device term and statuses")
	}
	v := url.Values{}
	v.Set("query.intr", device)
	v.Set("filter.advanced", "AREA[InterventionType]DEVICE")
	v.Set("filter.overallStatus", strings.Join(statuses, "|"))
	v.Set("fields", "NCTId")
	v.Set("pageSize", "1")
	v.Set("countTotal", "true")
	body, _, err := s.client.GetJSON(ctx, "/api/v2/studies", v)
	if err != nil {
		return 0, err
	}
	var env struct {
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, err
	}
	return env.TotalCount, nil
}

// MeshLookup is the optional CT.gov capability Module 07 clustering uses:
// MeSH condition terms for a device's trials, and the device interventions
// studied under a condition.
type MeshLookup interface {
	MeshTerms(ctx context.Context, device string, maxTrials int) ([]string, error)
	InterventionsForCondition(ctx context.Context, condition string, limit int) ([]string, error)
}

// MeshTerms returns the MeSH condition terms attached to the device's trials
// (fields=NCTId|ConditionMeshTerm, verified live 2026-07-09), most frequent
// first.
func (s *ClinicalTrials) MeshTerms(ctx context.Context, device string, maxTrials int) ([]string, error) {
	if device == "" {
		return nil, fmt.Errorf("mesh terms require a device term")
	}
	if maxTrials < 1 {
		maxTrials = 20
	}
	v := url.Values{}
	v.Set("query.intr", device)
	v.Set("filter.advanced", "AREA[InterventionType]DEVICE")
	v.Set("fields", "NCTId|ConditionMeshTerm")
	v.Set("pageSize", strconv.Itoa(maxTrials))
	body, _, err := s.client.GetJSON(ctx, "/api/v2/studies", v)
	if err != nil {
		return nil, err
	}
	var env struct {
		Studies []struct {
			DerivedSection struct {
				ConditionBrowseModule struct {
					Meshes []struct {
						Term string `json:"term"`
					} `json:"meshes"`
				} `json:"conditionBrowseModule"`
			} `json:"derivedSection"`
		} `json:"studies"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	freq := map[string]int{}
	var order []string
	for _, st := range env.Studies {
		for _, m := range st.DerivedSection.ConditionBrowseModule.Meshes {
			if m.Term == "" {
				continue
			}
			if freq[m.Term] == 0 {
				order = append(order, m.Term)
			}
			freq[m.Term]++
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return freq[order[i]] > freq[order[j]] })
	return order, nil
}

// InterventionsForCondition returns the device-intervention names studied
// under a MeSH condition (query.cond, verified live 2026-07-09), deduplicated
// in response order.
func (s *ClinicalTrials) InterventionsForCondition(ctx context.Context, condition string, limit int) ([]string, error) {
	if condition == "" {
		return nil, fmt.Errorf("intervention lookup requires a condition")
	}
	if limit < 1 {
		limit = 20
	}
	v := url.Values{}
	v.Set("query.cond", `"`+condition+`"`)
	v.Set("filter.advanced", "AREA[InterventionType]DEVICE")
	v.Set("fields", "NCTId|InterventionName")
	v.Set("pageSize", strconv.Itoa(limit))
	body, _, err := s.client.GetJSON(ctx, "/api/v2/studies", v)
	if err != nil {
		return nil, err
	}
	var env struct {
		Studies []struct {
			ProtocolSection struct {
				ArmsInterventionsModule struct {
					Interventions []struct {
						Name string `json:"name"`
					} `json:"interventions"`
				} `json:"armsInterventionsModule"`
			} `json:"protocolSection"`
		} `json:"studies"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, st := range env.Studies {
		for _, iv := range st.ProtocolSection.ArmsInterventionsModule.Interventions {
			if iv.Name == "" || seen[iv.Name] {
				continue
			}
			seen[iv.Name] = true
			out = append(out, iv.Name)
		}
	}
	return out, nil
}

func (s *ClinicalTrials) Health(ctx context.Context) error {
	v := url.Values{}
	v.Set("pageSize", "1")
	v.Set("fields", "NCTId")
	_, _, err := s.client.GetJSON(ctx, "/api/v2/studies", v)
	return err
}

func init() { Register(NewClinicalTrials()) }
