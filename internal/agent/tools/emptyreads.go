package tools

import (
	"context"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	diagnosisTimeout  = 4 * time.Second
	maxDiagnosisCalls = 6
	diagnosisAround   = 7 * 24 * time.Hour
	maxDiagnosisSpan  = 30 * 24 * time.Hour
)

func aroundWindow(rng datasource.TimeRange, now time.Time) datasource.TimeRange {
	to := rng.To
	if now.After(to) {
		to = now
	}
	if limit := rng.To.Add(diagnosisAround); to.After(limit) {
		to = limit
	}
	from := rng.From.Add(-diagnosisAround)
	if limit := to.Add(-maxDiagnosisSpan); from.Before(limit) {
		from = limit
	}
	return datasource.TimeRange{From: from, To: to}
}

func (r *Registry) explainEmpty(ctx context.Context, inst datasource.Instance, query string, rng datasource.TimeRange) string {
	store := typeOf(inst)
	sels := emptySelectors(store, query)
	if len(sels) == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosisTimeout)
	defer cancel()
	reader := &factReader{r: r, inst: inst, uid: inst.UID, rng: rng, around: aroundWindow(rng, Now(ctx)), calls: maxDiagnosisCalls, seen: map[queryproxy.NamesRequest]nameRead{}}
	allExist := true
	for _, sel := range sels {
		d := reader.diagnose(ctx, store, sel)
		if d.note != "" {
			return d.note
		}
		allExist = allExist && d.allExists
	}
	if allExist && store == spec.Prometheus {
		return allExistNote(sels)
	}
	return ""
}

func executedText(s querySummary, query string) string {
	if len(s.Executed) == 1 {
		return s.Executed[0].Text
	}
	return query
}

type factReader struct {
	r      *Registry
	inst   datasource.Instance
	uid    string
	rng    datasource.TimeRange
	around datasource.TimeRange
	calls  int
	seen   map[queryproxy.NamesRequest]nameRead
}

func (fr *factReader) diagnose(ctx context.Context, store spec.DatasourceType, sel emptySelector) diagnosis {
	f := emptyFacts{values: map[string]nameRead{}, ever: map[string]nameRead{}}
	for {
		d := diagnoseSelector(store, sel, f)
		if d.need.kind == factNone {
			return d
		}
		var got nameRead
		var ok bool
		if d.need.kind == factMatches {
			got, ok = fr.countLines(ctx, sel)
		} else {
			got, ok = fr.read(ctx, fr.request(sel, d.need))
		}
		if !ok {
			return diagnosis{}
		}
		f.set(d.need, got)
	}
}

func (f *emptyFacts) set(need factNeed, got nameRead) {
	switch need.kind {
	case factKeys:
		f.keys = got
	case factKeysEver:
		f.keysEver = got
	case factMetrics:
		f.metrics = got
	case factMatches:
		f.matches = got
	case factValues:
		f.values[need.key] = got
	case factValuesEver:
		f.ever[need.key] = got
	}
}

func (fr *factReader) request(sel emptySelector, need factNeed) queryproxy.NamesRequest {
	req := queryproxy.NamesRequest{Datasource: fr.uid, Kind: datasource.DiscoverLabels, Metric: sel.metric, Limit: queryproxy.MaxNamesLimit, Range: fr.rng}
	switch need.kind {
	case factKeysEver:
		req.Range = fr.around
	case factMetrics:
		req.Kind, req.Label, req.Metric, req.Search, req.Range = datasource.DiscoverLabelValues, metricNameLabel, "", metricStem(sel.metric), fr.around
	case factValues, factValuesEver:
		req.Kind, req.Label = datasource.DiscoverLabelValues, need.key
		if need.kind == factValuesEver {
			req.Range = fr.around
		}
	}
	return req
}

func metricStem(name string) string {
	for _, tok := range tokens(name) {
		if len(tok) > 2 {
			return tok
		}
	}
	return name
}

func (fr *factReader) read(ctx context.Context, req queryproxy.NamesRequest) (nameRead, bool) {
	if got, ok := fr.seen[req]; ok {
		return got, true
	}
	if fr.calls == 0 || ctx.Err() != nil {
		return nameRead{}, false
	}
	fr.calls--
	names, err := fr.r.deps.Runner.Names(ctx, req)
	got := nameRead{done: true, failed: err != nil, truncated: names.Truncated}
	if req.Kind == datasource.DiscoverLabels {
		got.names = names.Labels
	} else {
		got.names = names.Values
	}
	fr.seen[req] = got
	return got, true
}

func (fr *factReader) countLines(ctx context.Context, sel emptySelector) (nameRead, bool) {
	if fr.calls == 0 || ctx.Err() != nil {
		return nameRead{}, false
	}
	fr.calls--
	window := fr.rng.To.Sub(fr.rng.From).Truncate(time.Second)
	query := lokiSum(nil, "count_over_time("+sel.text+" ["+promSeconds(max(window, time.Second))+"])")
	frames, failed, err := fr.r.measureRun(ctx, fr.inst, query, fr.rng)
	if failed != nil || err != nil {
		return nameRead{done: true, failed: true}, true
	}
	got := nameRead{done: true}
	if values, _ := aggregatedValues(frames); len(values) == 1 && values[0].Value != nil && *values[0].Value > 0 {
		got.names = []string{strconv.FormatFloat(*values[0].Value, 'f', -1, 64)}
	}
	return got, true
}

func explainedNote(diagnosis, note string) string {
	if diagnosis == "" {
		return note
	}
	return joinNotes(diagnosis, note) + "."
}

func emptyDiagnosable(inst datasource.Instance) bool {
	t := typeOf(inst)
	return t == spec.Prometheus || t == spec.Loki
}
