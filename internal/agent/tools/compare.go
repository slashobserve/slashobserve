package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/tracecompare"
)

const (
	nameCompareTraces   = "compare_traces"
	compareSourcesText  = "compare_traces reads Tempo and ClickHouse datasources"
	compareDefaultLimit = 20
	compareMaxLimit     = 100

	descCompareTraces   = "Compare two sets of traces by id and get the attribute values (service, span name, status, kind, span and resource attributes) whose share of traces differs most, ranked by the gap. Use it after listing traces to explain why some are slow or failing: pass the slow or failing ids as selected and the rest as baseline."
	schemaCompareTraces = `{"type":"object","properties":{` + propDatasource + `,` +
		`"selected":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":50,"description":"The trace ids to explain, 16 or 32 hex characters each."},` +
		`"baseline":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":150,"description":"The trace ids to compare against, such as the other traces listed."},` +
		propTraceFrom + `,` + propTo + `,` +
		`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"The most differences wanted. Default 20."}},` +
		`"required":["datasource","selected","baseline"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).compareEntries) }

func (r *Registry) compareEntries() []entry {
	return []entry{
		{Tool{Name: nameCompareTraces, Description: descCompareTraces, InputSchema: json.RawMessage(schemaCompareTraces), ReadOnly: true, MinRole: auth.RoleViewer}, r.compareTraces},
	}
}

type traceCompareOut struct {
	Selected    traceSetOut     `json:"selected"`
	Baseline    traceSetOut     `json:"baseline"`
	Attributes  int             `json:"attributes"`
	Partial     bool            `json:"partial"`
	Differences []differenceOut `json:"differences"`
	Truncated   bool            `json:"truncated"`
}

type traceSetOut struct {
	Requested int `json:"requested"`
	Read      int `json:"read"`
}

type differenceOut struct {
	Attribute     string  `json:"attribute"`
	Value         string  `json:"value"`
	SelectedShare float64 `json:"selected_share"`
	BaselineShare float64 `json:"baseline_share"`
	SelectedCount int     `json:"selected_count"`
	BaselineCount int     `json:"baseline_count"`
}

func (r *Registry) compareTraces(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string   `json:"datasource"`
		Selected   []string `json:"selected"`
		Baseline   []string `json:"baseline"`
		From       string   `json:"from"`
		To         string   `json:"to"`
		Limit      *int     `json:"limit"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	selected, baseline, err := datasource.CompareTraceIDs(in.Selected, in.Baseline)
	if err != nil {
		return invalid("%s", err.Error()), nil
	}
	limit, res, ok := boundedInt(in.Limit, compareDefaultLimit, 1, compareMaxLimit, "limit")
	if !ok {
		return res, nil
	}
	source, ok := inst.Datasource.(datasource.TraceAttributeSource)
	if !ok {
		return invalid("%s", compareSourcesText), nil
	}
	rng, res, ok := resolveRange(cmp.Or(in.From, traceDefaultFrom), in.To, time.Now())
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	step := "Compare traces on " + inst.UID
	release, err := r.acquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	selectedSets, err := source.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: selected, Range: rng, MaxSpans: datasource.CompareSpanBudget})
	if err != nil {
		return datasourceFailure(step, err)
	}
	baselineSets, err := source.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: baseline, Range: rng, MaxSpans: datasource.CompareSpanBudget})
	if err != nil {
		return datasourceFailure(step, err)
	}
	out := newTraceCompareOut(len(selected), selectedSets, len(baseline), baselineSets, tracecompare.Compare(selectedSets, baselineSets, limit))
	text := fmt.Sprintf("Compared %d selected with %s on %s: %s", out.Selected.Read, plural(out.Baseline.Read, "other trace", "other traces"), inst.UID, plural(len(out.Differences), "difference", "differences"))
	return success(text, out), nil
}

func newTraceCompareOut(selectedRequested int, selected []datasource.TraceAttributeSet, baselineRequested int, baseline []datasource.TraceAttributeSet, result tracecompare.Result) traceCompareOut {
	out := traceCompareOut{
		Selected:    traceSetOut{Requested: selectedRequested, Read: len(selected)},
		Baseline:    traceSetOut{Requested: baselineRequested, Read: len(baseline)},
		Attributes:  result.Attributes,
		Partial:     len(selected) < selectedRequested || len(baseline) < baselineRequested,
		Differences: make([]differenceOut, len(result.Differences)),
		Truncated:   result.Truncated,
	}
	for i, d := range result.Differences {
		out.Differences[i] = differenceOut{
			Attribute:     d.Attribute,
			Value:         d.Value,
			SelectedShare: d.SelectedShare,
			BaselineShare: d.BaselineShare,
			SelectedCount: d.SelectedCount,
			BaselineCount: d.BaselineCount,
		}
	}
	return out
}
