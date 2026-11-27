package findings

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	MaxCauses        = 6
	MaxAffected      = 8
	MaxCauseFacts    = 12
	MaxCauseTraceIDs = 4
	MaxCauseQueries  = 6
	MaxKeyRunes      = 200
	maxQueryBytes    = 400

	RelationIndependent = "independent"
	RelationKnockOn     = "knock_on"
)

var (
	ComponentKinds = []string{"service", "pod", "deployment", "config", "flag", "datastore", "dependency", "node", "other"}
	TriggerKinds   = []string{"feature_flag", "config_value", "deploy", "traffic", "dependency", "unknown"}

	factID       = regexp.MustCompile(`^f[0-9]{1,6}$`)
	causeTraceID = regexp.MustCompile(`^(?:[0-9a-fA-F]{32}|[0-9a-fA-F]{16})$`)
)

type Component struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (c Component) Text() string {
	if c.Namespace == "" {
		return c.Name
	}
	return c.Namespace + "/" + c.Name
}

type Trigger struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

type CauseEvidence struct {
	Facts    []string `json:"facts,omitempty"`
	TraceIDs []string `json:"trace_ids,omitempty"`
	Queries  []string `json:"queries,omitempty"`
}

type Cause struct {
	Component Component     `json:"component"`
	Kind      string        `json:"kind"`
	Mechanism string        `json:"mechanism"`
	Trigger   *Trigger      `json:"trigger,omitempty"`
	Onset     *time.Time    `json:"onset,omitempty"`
	Affected  []Component   `json:"affected,omitempty"`
	KnockOnOf *int          `json:"knock_on_of,omitempty"`
	Evidence  CauseEvidence `json:"evidence"`
	Estimate  string        `json:"estimate,omitempty"`
	Score     *float64      `json:"score,omitempty"`
	Band      string        `json:"band,omitempty"`
	Stated    bool          `json:"stated"`
}

func (c Cause) Relation() string {
	if c.KnockOnOf != nil {
		return RelationKnockOn
	}
	return RelationIndependent
}

func (c Cause) rootCause() *RootCause {
	return &RootCause{Component: c.Component.Text(), Kind: c.Kind, Summary: c.Mechanism}
}

type ComponentInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type TriggerInput struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EvidenceInput struct {
	Facts    []string `json:"facts"`
	TraceIDs []string `json:"trace_ids"`
}

type CauseInput struct {
	Component ComponentInput   `json:"component"`
	Kind      string           `json:"kind"`
	Mechanism string           `json:"mechanism"`
	Trigger   *TriggerInput    `json:"trigger"`
	Onset     string           `json:"onset"`
	Affected  []ComponentInput `json:"affected"`
	KnockOnOf *int             `json:"knock_on_of"`
	Evidence  EvidenceInput    `json:"evidence"`
	Estimate  string           `json:"estimate"`
	CauseGiven
}

type CauseGiven struct {
	Signals *GivenSignals `json:"signals"`
	Score   *float64      `json:"score"`
	Band    string        `json:"band"`
	Queries []string      `json:"queries"`
}

type GivenSignals struct {
	Kinds       []string `json:"kinds"`
	Component   bool     `json:"component"`
	OnsetRead   bool     `json:"onset_read"`
	OnsetFact   bool     `json:"onset_fact"`
	TriggerRead bool     `json:"trigger_read"`
	RuledOut    bool     `json:"ruled_out"`
}

func (g GivenSignals) Signals() Signals {
	kinds := slices.DeleteFunc(slices.Clone(g.Kinds), func(k string) bool {
		return k != SignalMetrics && k != SignalLogs && k != SignalTraces
	})
	return Signals{Kinds: kinds, Component: g.Component, OnsetRead: g.OnsetRead, OnsetFact: g.OnsetFact, TriggerRead: g.TriggerRead, RuledOut: g.RuledOut}
}

func (in StructuredInput) CauseInputs() []CauseInput {
	if len(in.Causes) > 0 || in.RootCause == nil {
		return in.Causes
	}
	r := in.RootCause
	return []CauseInput{{
		Component: ComponentInput{Kind: "other", Name: r.Component},
		Kind:      r.Kind,
		Mechanism: r.Summary,
		Onset:     in.Onset,
		Evidence:  EvidenceInput{Facts: in.LegacyFacts},
		Estimate:  in.Confidence,
	}}
}

func (in StructuredInput) checkCauses(rootCause *RootCause) ([]Cause, error) {
	raw := in.CauseInputs()
	switch {
	case len(raw) > MaxCauses:
		return nil, fmt.Errorf("causes takes at most %d causes; list only those the evidence supports", MaxCauses)
	case in.NoIncident && len(raw) > 0:
		return nil, fmt.Errorf("no_incident is true, so causes must be left out")
	}
	causes := make([]Cause, len(raw))
	for i, c := range raw {
		cause, err := checkCause(i, c, len(raw))
		if err != nil {
			return nil, err
		}
		causes[i] = cause
	}
	for i, c := range causes {
		if c.KnockOnOf != nil && causes[*c.KnockOnOf].KnockOnOf != nil {
			return nil, fmt.Errorf("causes[%d].knock_on_of must name a cause that is not itself a knock-on", i)
		}
	}
	if len(in.Causes) > 0 && rootCause != nil && !agrees(*rootCause, causes[0]) {
		return nil, fmt.Errorf("root_cause must describe causes[0], with the same component and kind; send only causes, and root_cause is filled from the first stated cause")
	}
	if len(causes) == 0 {
		return nil, nil
	}
	return causes, nil
}

func agrees(r RootCause, c Cause) bool {
	component := strings.ToLower(r.Component)
	return r.Kind == c.Kind && (component == strings.ToLower(c.Component.Name) || component == strings.ToLower(c.Component.Text()))
}

func checkCause(i int, in CauseInput, count int) (Cause, error) {
	at := fmt.Sprintf("causes[%d]", i)
	component, err := checkComponent(at+".component", in.Component)
	if err != nil {
		return Cause{}, err
	}
	out := Cause{Component: component, Stated: true}
	if out.Mechanism, err = sentence(at+".mechanism", in.Mechanism, MaxSentenceRunes); err != nil {
		return Cause{}, err
	}
	out.Kind = strings.TrimSpace(in.Kind)
	switch {
	case out.Mechanism == "":
		return Cause{}, fmt.Errorf("%s.mechanism is required: one sentence on how the component failed", at)
	case !slices.Contains(RootCauseKinds, out.Kind):
		return Cause{}, fmt.Errorf("%s.kind must be one of %s", at, strings.Join(RootCauseKinds, ", "))
	}
	if out.Trigger, err = checkTrigger(at+".trigger", in.Trigger); err != nil {
		return Cause{}, err
	}
	if out.Onset, err = onsetOf(at+".onset", in.Onset); err != nil {
		return Cause{}, err
	}
	if out.Affected, err = checkAffected(at+".affected", in.Affected); err != nil {
		return Cause{}, err
	}
	if out.KnockOnOf, err = checkKnockOn(at+".knock_on_of", in.KnockOnOf, i, count); err != nil {
		return Cause{}, err
	}
	if out.Evidence, err = checkEvidence(at+".evidence", in.Evidence); err != nil {
		return Cause{}, err
	}
	if out.Estimate = strings.TrimSpace(in.Estimate); out.Estimate != "" && !slices.Contains(Confidences, out.Estimate) {
		return Cause{}, fmt.Errorf("%s.estimate must be one of %s", at, strings.Join(Confidences, ", "))
	}
	return out, nil
}

func checkAffected(at string, in []ComponentInput) ([]Component, error) {
	if len(in) > MaxAffected {
		return nil, fmt.Errorf("%s takes at most %d components", at, MaxAffected)
	}
	var out []Component
	for j, a := range in {
		c, err := checkComponent(fmt.Sprintf("%s[%d]", at, j), a)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func checkKnockOn(at string, k *int, self, count int) (*int, error) {
	if k == nil {
		return nil, nil
	}
	if *k == self || *k < 0 || *k >= count {
		return nil, fmt.Errorf("%s must be the index of another listed cause, from 0 to %d, not %d", at, count-1, *k)
	}
	return new(*k), nil
}

func checkComponent(at string, in ComponentInput) (Component, error) {
	name, err := sentence(at+".name", in.Name, MaxComponentRunes)
	if err != nil {
		return Component{}, err
	}
	namespace, err := sentence(at+".namespace", in.Namespace, MaxComponentRunes)
	if err != nil {
		return Component{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	switch {
	case name == "":
		return Component{}, fmt.Errorf("%s.name is required, as the telemetry names it", at)
	case !slices.Contains(ComponentKinds, kind):
		return Component{}, fmt.Errorf("%s.kind must be one of %s", at, strings.Join(ComponentKinds, ", "))
	}
	return Component{Kind: kind, Name: name, Namespace: namespace}, nil
}

func checkTrigger(at string, in *TriggerInput) (*Trigger, error) {
	if in == nil {
		return nil, nil
	}
	key, err := sentence(at+".key", in.Key, MaxKeyRunes)
	if err != nil {
		return nil, err
	}
	value, err := sentence(at+".value", in.Value, MaxKeyRunes)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(in.Kind)
	switch {
	case !slices.Contains(TriggerKinds, kind):
		return nil, fmt.Errorf("%s.kind must be one of %s", at, strings.Join(TriggerKinds, ", "))
	case key == "":
		return nil, fmt.Errorf("%s.key is required: the flag key, setting, version or name exactly as the system spells it", at)
	}
	return &Trigger{Kind: kind, Key: key, Value: value}, nil
}

func checkEvidence(at string, in EvidenceInput) (CauseEvidence, error) {
	var out CauseEvidence
	if len(in.Facts) > MaxCauseFacts {
		return out, fmt.Errorf("%s.facts takes at most %d fact ids", at, MaxCauseFacts)
	}
	if len(in.TraceIDs) > MaxCauseTraceIDs {
		return out, fmt.Errorf("%s.trace_ids takes at most %d trace ids", at, MaxCauseTraceIDs)
	}
	for _, id := range in.Facts {
		id = strings.TrimSpace(id)
		if !factID.MatchString(id) {
			return out, fmt.Errorf("%s.facts takes fact ids such as f3 from the results this run read", at)
		}
		if !slices.Contains(out.Facts, id) {
			out.Facts = append(out.Facts, id)
		}
	}
	for _, id := range in.TraceIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if !causeTraceID.MatchString(id) {
			return out, fmt.Errorf("%s.trace_ids takes 16 or 32 hex digit trace ids", at)
		}
		if !slices.Contains(out.TraceIDs, id) {
			out.TraceIDs = append(out.TraceIDs, id)
		}
	}
	return out, nil
}

func onsetOf(at, raw string) (*time.Time, error) {
	onset := strings.TrimSpace(raw)
	if onset == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, onset)
	if err != nil {
		return nil, fmt.Errorf("%s must be an absolute RFC 3339 time such as 2026-11-20T10:05:00Z", at)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func (s *Structured) settle() {
	s.RootCause = nil
	for _, c := range s.Causes {
		if c.Stated && c.KnockOnOf == nil {
			s.RootCause = c.rootCause()
			return
		}
	}
}

func (s *Structured) GiveQueries(i int, queries []string) {
	if i < 0 || i >= len(s.Causes) {
		return
	}
	e := &s.Causes[i].Evidence
	e.Queries = nil
	for _, raw := range queries {
		q := clipQuery(strings.TrimSpace(raw))
		if q != "" && !slices.Contains(e.Queries, q) && len(e.Queries) < MaxCauseQueries {
			e.Queries = append(e.Queries, q)
		}
	}
}

func (s *Structured) WithQueries(queryOf func(factID string) string) {
	for i := range s.Causes {
		e := &s.Causes[i].Evidence
		if len(e.Queries) > 0 {
			continue
		}
		for _, id := range e.Facts {
			q := clipQuery(strings.TrimSpace(queryOf(id)))
			if q != "" && !slices.Contains(e.Queries, q) && len(e.Queries) < MaxCauseQueries {
				e.Queries = append(e.Queries, q)
			}
		}
	}
}

func clipQuery(q string) string {
	if len(q) <= maxQueryBytes {
		return q
	}
	n := maxQueryBytes
	for n > 0 && q[n]&0xC0 == 0x80 {
		n--
	}
	return q[:n]
}

func encodeCauses(causes []Cause) ([]byte, error) {
	if len(causes) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(causes)
	if err != nil {
		return nil, fmt.Errorf("encode causes: %w", err)
	}
	return raw, nil
}

func causesOf(raw []byte, legacy *RootCause, onset *time.Time, confidence string) []Cause {
	var causes []Cause
	if len(raw) > 0 && json.Unmarshal(raw, &causes) == nil {
		return causes
	}
	if legacy == nil {
		return nil
	}
	return []Cause{{
		Component: Component{Kind: "other", Name: legacy.Component},
		Kind:      legacy.Kind,
		Mechanism: legacy.Summary,
		Onset:     onset,
		Estimate:  confidence,
		Stated:    true,
	}}
}
