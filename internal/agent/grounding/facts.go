package grounding

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	MaxFacts       = 400
	maxFactsOfRead = 120
	maxFactLabels  = 12
	maxFactQuery   = 400
	maxFactText    = 64
	maxIDs         = 1000

	factsKey = "facts"
)

type FactKind string

const (
	FactNumber   FactKind = "number"
	FactDuration FactKind = "duration"
	FactCount    FactKind = "count"
	FactRatio    FactKind = "ratio"
	FactTraceID  FactKind = "trace_id"
)

type Fact struct {
	ID     string            `json:"id"`
	Text   string            `json:"text"`
	Value  float64           `json:"value,omitempty"`
	Kind   FactKind          `json:"kind"`
	Unit   string            `json:"unit,omitempty"`
	Of     string            `json:"of"`
	Labels map[string]string `json:"labels,omitempty"`
	At     string            `json:"at,omitempty"`
	From   string            `json:"from,omitempty"`
	To     string            `json:"to,omitempty"`
	Query  string            `json:"query,omitempty"`
}

func (f Fact) same(g Fact) bool {
	return f.Of == g.Of && f.Text == g.Text && f.At == g.At && f.From == g.From && f.To == g.To && f.Query == g.Query && maps.Equal(f.Labels, g.Labels)
}

type factSource map[string][]string

var factSources = map[string]factSource{
	"measure_window":    {"values[]": {"value", "seconds_after_from"}},
	"measure_aggregate": {"values[]": {"value", "numerator", "denominator"}},
	"trace_stats": {
		"":         {"spans", "errors", "quantile_spans"},
		"window":   {"p50_ms", "p95_ms", "p99_ms"},
		"groups[]": {"spans", "errors", "p50_ms", "p95_ms", "p99_ms", "quantile_spans"},
	},
	"trace_error_origins": {
		"":                      {"failing_traces", "examined", "unattributed"},
		"routes[]":              {"failing_traces", "examined"},
		"groups[]":              {"traces", "share", "example_trace_id"},
		"groups[].relayed_by[]": {"traces"},
	},
	"trace_slow_path": {
		"":                         {"listed_traces"},
		"routes[]":                 {"threshold_ms", "p50_ms", "slow_traces", "examined"},
		"routes[].top_self_time[]": {"self_time_ms", "share", "traces"},
		"routes[].services[]":      {"traces"},
		"routes[].example":         {"trace_id", "duration_ms"},
	},
	"get_trace": {
		"":           {"trace_id", "spans", "duration_ms"},
		"services[]": {"spans", "errors"},
		"slowest[]":  {"duration_ms", "self_time_ms"},
	},
	"query_range":   {"series[]": {"min", "max", "last"}},
	"query_instant": {"series[]": {"last"}},
}

func IssuesFacts(tool string) bool {
	return factSources[tool] != nil
}

var labelFields = map[string]bool{"route": true, "service": true, "operation": true, "name": true, "trace_id": true, "status_message": true, "span_id": true}

var countFields = map[string]bool{
	"spans": true, "errors": true, "traces": true, "failing_traces": true, "slow_traces": true,
	"examined": true, "listed_traces": true, "unattributed": true, "quantile_spans": true,
}

var factQueries = map[string]string{"numerator": "numerator_query", "denominator": "denominator_query"}

func kindOf(key string) (FactKind, string) {
	switch {
	case key == "trace_id" || key == "example_trace_id":
		return FactTraceID, ""
	case strings.HasSuffix(key, "_ms"):
		return FactDuration, "ms"
	case key == "seconds_after_from":
		return FactDuration, "s"
	case key == "share":
		return FactRatio, ""
	case countFields[key]:
		return FactCount, ""
	}
	return FactNumber, ""
}

type issuing struct {
	record  *Record
	tool    string
	source  factSource
	top     *node
	from    string
	to      string
	query   string
	qualify string
	issued  int
}

func (r *Record) IssueFacts(tool string, input, content []byte, ranges []Range) []byte {
	source := factSources[tool]
	if r == nil || source == nil {
		return content
	}
	top, ok := parseJSON(content)
	if !ok || top.kind != nodeObject {
		return content
	}
	is := issuing{record: r, tool: tool, source: source, top: top}
	is.window(ranges)
	is.query = top.str("query")
	if is.query == "" {
		if executed := top.get("executed"); executed != nil && len(executed.vals) > 0 {
			is.query = executed.vals[0].str("text")
		}
	}
	if is.query == "" {
		if in, ok := parseJSON(input); ok && in.kind == nodeObject {
			is.query = in.str("query")
		}
	}
	is.qualify = strings.TrimSpace(top.str("stat") + " " + top.str("op"))
	is.visit(top, "", nil)
	if is.issued == 0 {
		return content
	}
	return top.encode()
}

func (is *issuing) window(ranges []Range) {
	from, to := is.top.str("from"), is.top.str("to")
	if _, err := time.Parse(time.RFC3339Nano, from); err == nil {
		if _, err := time.Parse(time.RFC3339Nano, to); err == nil {
			is.from, is.to = from, to
			return
		}
	}
	if len(ranges) > 0 {
		is.from, is.to = ranges[0].From.UTC().Format(time.RFC3339), ranges[0].To.UTC().Format(time.RFC3339)
	}
}

func (is *issuing) visit(n *node, path string, inherited map[string]string) {
	labels := is.labelsOf(n, path, inherited)
	if keys, ok := is.source[path]; ok && n.get(factsKey) == nil {
		ids := &node{kind: nodeObject}
		for _, key := range keys {
			if id := is.fact(n, path, key, labels); id != "" {
				ids.set(key, stringNode(id))
			}
		}
		if len(ids.keys) > 0 {
			n.set(factsKey, ids)
		}
	}
	for i, key := range n.keys {
		child := n.vals[i]
		at := key
		if path != "" {
			at = path + "." + key
		}
		switch child.kind {
		case nodeObject:
			is.visit(child, at, labels)
		case nodeArray:
			for _, item := range child.vals {
				if item.kind == nodeObject {
					is.visit(item, at+"[]", labels)
				}
			}
		}
	}
}

func (is *issuing) labelsOf(n *node, path string, inherited map[string]string) map[string]string {
	labels := maps.Clone(inherited)
	add := func(key, value string) {
		if labels == nil {
			labels = map[string]string{}
		}
		if len(labels) < maxFactLabels && value != "" && len(value) <= maxNameBytes {
			labels[key] = value
		}
	}
	if own := n.get("labels"); own != nil && own.kind == nodeObject {
		for i, key := range own.keys {
			if own.vals[i].kind == nodeString {
				add(key, own.vals[i].text)
			}
		}
	}
	for i, key := range n.keys {
		if n.vals[i].kind != nodeString {
			continue
		}
		switch {
		case labelFields[key]:
			add(key, n.vals[i].text)
		case key == "value" && path == "groups[]" && is.top.str("group_by") != "":
			add(is.top.str("group_by"), n.vals[i].text)
		}
	}
	return labels
}

func (is *issuing) fact(n *node, path, key string, labels map[string]string) string {
	value := n.get(key)
	if value == nil || is.issued >= maxFactsOfRead {
		return ""
	}
	kind, unit := kindOf(key)
	f := Fact{Kind: kind, Unit: unit, Of: is.of(path, key), Labels: labels, At: n.str("at"), From: is.from, To: is.to, Query: clipBytes(is.query, maxFactQuery)}
	if q := factQueries[key]; q != "" {
		f.Query = clipBytes(is.top.str(q), maxFactQuery)
	}
	switch {
	case kind == FactTraceID && value.kind == nodeString && traceID.MatchString(value.text):
		f.Text = strings.ToLower(value.text)
		f.Labels = withoutLabel(labels, "trace_id")
	case kind != FactTraceID && value.kind == nodeNumber && len(value.text) <= maxFactText:
		v, err := strconv.ParseFloat(value.text, 64)
		if err != nil {
			return ""
		}
		f.Text, f.Value = value.text, v
	default:
		return ""
	}
	is.issued++
	return is.record.addFact(f)
}

func (is *issuing) of(path, key string) string {
	parts := []string{is.tool}
	if is.qualify != "" {
		parts = append(parts, is.qualify)
	}
	if path != "" {
		parts = append(parts, path)
	}
	return strings.Join(append(parts, key), " ")
}

func withoutLabel(labels map[string]string, key string) map[string]string {
	if _, ok := labels[key]; !ok {
		return labels
	}
	out := maps.Clone(labels)
	delete(out, key)
	if len(out) == 0 {
		return nil
	}
	return out
}

func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func (r *Record) addFact(f Fact) string {
	if i := slices.IndexFunc(r.Facts, f.same); i >= 0 {
		return r.Facts[i].ID
	}
	r.NextFact++
	f.ID = "f" + strconv.Itoa(r.NextFact)
	r.Facts = append(r.Facts, f)
	if over := len(r.Facts) - MaxFacts; over > 0 {
		r.Facts = slices.Delete(r.Facts, 0, over)
	}
	return f.ID
}

func (r *Record) Fact(id string) (Fact, bool) {
	if r == nil {
		return Fact{}, false
	}
	i := slices.IndexFunc(r.Facts, func(f Fact) bool { return f.ID == id })
	if i < 0 {
		return Fact{}, false
	}
	return r.Facts[i], true
}

func (r *Record) addID(id string) {
	id = strings.ToLower(id)
	if i := slices.Index(r.IDs, id); i >= 0 {
		return
	}
	r.IDs = append(r.IDs, id)
	if over := len(r.IDs) - maxIDs; over > 0 {
		r.IDs = slices.Delete(r.IDs, 0, over)
	}
}
