package elasticsearch

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type aggKind int

const (
	aggValue aggKind = iota
	aggPercentiles
	aggStats
	aggBucket
	aggTimeBucket
)

var (
	timeBucketTypes  = []string{"date_histogram", "auto_date_histogram"}
	bucketTypes      = []string{"histogram", "terms", "multi_terms", "filters", "range", "date_range", "significant_terms"}
	valueMetricTypes = []string{"avg", "sum", "min", "max", "value_count", "cardinality", "median_absolute_deviation", "rate", "bucket_script", "derivative", "cumulative_sum", "moving_fn"}
	statsKeys        = []string{"avg", "sum", "min", "max", "count"}
)

type aggNode struct {
	name, typ, column string
	kind              aggKind
	children          []aggNode
}

func (n aggNode) isBucket() bool { return n.kind == aggBucket || n.kind == aggTimeBucket }

func unsupportedAgg(name, typ, why string) error {
	msg := fmt.Sprintf("unsupported aggregation %q (type %s)", truncateText(name), truncateText(typ))
	if why != "" {
		msg += ": " + why
	}
	return datasource.Errorf(datasource.ErrInvalidQuery, "%s", msg)
}

func parseAggTree(tree map[string]any, timeField string, underTime bool) ([]aggNode, error) {
	nodes := make([]aggNode, 0, len(tree))
	buckets := 0
	for _, name := range slices.Sorted(maps.Keys(tree)) {
		node, err := parseAgg(name, tree[name], timeField, underTime)
		if err != nil {
			return nil, err
		}
		if node.isBucket() {
			buckets++
		}
		if buckets > 1 {
			return nil, unsupportedAgg(node.name, node.typ, "one bucket aggregation per level")
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseAgg(name string, raw any, timeField string, underTime bool) (aggNode, error) {
	def, ok := raw.(map[string]any)
	if !ok {
		return aggNode{}, unsupportedAgg(name, "", "")
	}
	node, sub, err := splitAggDef(name, def)
	if err != nil {
		return aggNode{}, err
	}
	kind, ok := aggKindOf(node.typ, node.column, timeField)
	if !ok {
		return aggNode{}, unsupportedAgg(name, node.typ, "")
	}
	node.kind = kind
	if node.kind == aggTimeBucket && underTime {
		return aggNode{}, unsupportedAgg(name, node.typ, "a time bucket inside a time bucket")
	}
	if node.isBucket() && len(sub) > 0 {
		children, err := parseAggTree(sub, timeField, underTime || node.kind == aggTimeBucket)
		if err != nil {
			return aggNode{}, err
		}
		node.children = children
	}
	return node, nil
}

func splitAggDef(name string, def map[string]any) (aggNode, map[string]any, error) {
	var sub map[string]any
	node := aggNode{name: name, column: name}
	for _, key := range slices.Sorted(maps.Keys(def)) {
		switch key {
		case "aggs", "aggregations":
			sub, _ = def[key].(map[string]any)
		case "meta":
		default:
			if node.typ != "" {
				return aggNode{}, nil, unsupportedAgg(name, key, "one type per aggregation")
			}
			node.typ = key
			if params, ok := def[key].(map[string]any); ok {
				if field, ok := params["field"].(string); ok && field != "" {
					node.column = field
				}
			}
		}
	}
	return node, sub, nil
}

func aggKindOf(typ, column, timeField string) (aggKind, bool) {
	switch {
	case slices.Contains(timeBucketTypes, typ), typ == "histogram" && column == timeField:
		return aggTimeBucket, true
	case slices.Contains(bucketTypes, typ):
		return aggBucket, true
	case slices.Contains(valueMetricTypes, typ):
		return aggValue, true
	case typ == "percentiles":
		return aggPercentiles, true
	case typ == "stats", typ == "extended_stats":
		return aggStats, true
	}
	return 0, false
}

func hasTimeBucket(nodes []aggNode) bool {
	return slices.ContainsFunc(nodes, func(n aggNode) bool { return n.kind == aggTimeBucket || hasTimeBucket(n.children) })
}

type bucket struct {
	key      string
	keyMS    int64
	docCount float64
	subs     map[string]json.RawMessage
}

func parseBuckets(raw json.RawMessage) ([]bucket, error) {
	var wrapper struct {
		Buckets json.RawMessage `json:"buckets"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, unreadable()
	}
	trimmed := strings.TrimSpace(string(wrapper.Buckets))
	var entries []map[string]json.RawMessage
	switch {
	case trimmed == "" || trimmed == "null":
		return nil, nil
	case strings.HasPrefix(trimmed, "{"):
		var keyed map[string]map[string]json.RawMessage
		if err := json.Unmarshal(wrapper.Buckets, &keyed); err != nil {
			return nil, unreadable()
		}
		for _, name := range slices.Sorted(maps.Keys(keyed)) {
			entry := keyed[name]
			if _, ok := entry["key"]; !ok {
				quoted, _ := json.Marshal(name)
				entry["key"] = quoted
			}
			entries = append(entries, entry)
		}
	default:
		if err := json.Unmarshal(wrapper.Buckets, &entries); err != nil {
			return nil, unreadable()
		}
	}
	out := make([]bucket, 0, len(entries))
	for i, entry := range entries {
		if _, ok := entry["key"]; !ok {
			entry["key"] = json.RawMessage(strconv.Itoa(i))
		}
		b := bucket{subs: entry}
		var asString string
		if raw, ok := entry["key_as_string"]; ok {
			_ = json.Unmarshal(raw, &asString)
		}
		b.key = bucketKey(entry["key"], asString)
		var ms float64
		if json.Unmarshal(entry["key"], &ms) == nil {
			b.keyMS = int64(ms)
		}
		_ = json.Unmarshal(entry["doc_count"], &b.docCount)
		for _, name := range []string{"key", "key_as_string", "doc_count", "doc_count_error_upper_bound", "from", "from_as_string", "to", "to_as_string", "score", "bg_count"} {
			delete(entry, name)
		}
		out = append(out, b)
	}
	return out, nil
}

type metricValue struct {
	name  string
	key   string
	value any
}

func metricValues(node aggNode, raw json.RawMessage) ([]metricValue, error) {
	switch node.kind {
	case aggValue:
		var m struct {
			Value *float64 `json:"value"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, unreadable()
		}
		return []metricValue{{name: node.name, value: number(m.Value)}}, nil
	case aggStats:
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, unreadable()
		}
		out := make([]metricValue, 0, len(statsKeys))
		for _, k := range statsKeys {
			var v *float64
			_ = json.Unmarshal(m[k], &v)
			out = append(out, metricValue{name: node.name + " " + k, key: k, value: number(v)})
		}
		return out, nil
	}
	var m struct {
		Values json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, unreadable()
	}
	return percentileValues(node.name, m.Values)
}

func percentileValues(name string, raw json.RawMessage) ([]metricValue, error) {
	var out []metricValue
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var list []struct {
			Key   float64  `json:"key"`
			Value *float64 `json:"value"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, unreadable()
		}
		for _, p := range list {
			key := strconv.FormatFloat(p.Key, 'f', -1, 64)
			out = append(out, metricValue{name: name, key: key, value: number(p.Value)})
		}
		return out, nil
	}
	var keyed map[string]*float64
	if err := json.Unmarshal(raw, &keyed); err != nil {
		return nil, unreadable()
	}
	keys := slices.SortedFunc(maps.Keys(keyed), func(a, b string) int {
		fa, _ := strconv.ParseFloat(a, 64)
		fb, _ := strconv.ParseFloat(b, 64)
		return cmp.Compare(fa, fb)
	})
	for _, k := range keys {
		percent, _ := strconv.ParseFloat(k, 64)
		out = append(out, metricValue{name: name, key: strconv.FormatFloat(percent, 'f', -1, 64), value: number(keyed[k])})
	}
	return out, nil
}

func number(v *float64) any {
	if v == nil {
		return nil
	}
	return datasource.Number(*v)
}

type folder struct {
	lim   datasource.Limits
	total int
}

func foldAggregations(tree map[string]any, response map[string]json.RawMessage, total int, timeField string, lim datasource.Limits) ([]datasource.Frame, error) {
	nodes, err := parseAggTree(tree, timeField, false)
	if err != nil {
		return nil, err
	}
	f := &folder{lim: lim, total: total}
	if hasTimeBucket(nodes) {
		return f.series(nodes, response)
	}
	return f.table(nodes, response)
}

type seriesBuilder struct {
	keys   []string
	frames map[string]*datasource.Frame
	points int
}

func (f *folder) series(nodes []aggNode, response map[string]json.RawMessage) ([]datasource.Frame, error) {
	sb := &seriesBuilder{frames: map[string]*datasource.Frame{}}
	if err := f.walkSeries(sb, nodes, response, nil); err != nil {
		return nil, err
	}
	frames := make([]datasource.Frame, 0, len(sb.keys))
	for _, key := range sb.keys {
		frames = append(frames, *sb.frames[key])
	}
	slices.SortStableFunc(frames, func(a, b datasource.Frame) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(labelKey(a.Labels), labelKey(b.Labels))
	})
	return frames, nil
}

func (f *folder) walkSeries(sb *seriesBuilder, nodes []aggNode, subs map[string]json.RawMessage, labels map[string]string) error {
	for _, node := range nodes {
		raw, ok := subs[node.name]
		if !ok || !node.isBucket() {
			continue
		}
		buckets, err := parseBuckets(raw)
		if err != nil {
			return err
		}
		for _, b := range buckets {
			if node.kind == aggBucket {
				if err := f.walkSeries(sb, node.children, b.subs, withLabel(labels, node.column, b.key)); err != nil {
					return err
				}
				continue
			}
			leaves, err := f.leaves(node.children, b, labels)
			if err != nil {
				return err
			}
			for _, leaf := range leaves {
				if err := f.addPoint(sb, leaf, time.UnixMilli(b.keyMS).UTC()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type leaf struct {
	labels map[string]string
	name   string
	value  any
}

func (f *folder) leaves(nodes []aggNode, b bucket, labels map[string]string) ([]leaf, error) {
	if len(nodes) == 0 {
		return []leaf{{labels: labels, name: "count", value: datasource.Number(b.docCount)}}, nil
	}
	var out []leaf
	for _, node := range nodes {
		raw, ok := b.subs[node.name]
		if !ok {
			continue
		}
		if node.isBucket() {
			buckets, err := parseBuckets(raw)
			if err != nil {
				return nil, err
			}
			for _, child := range buckets {
				more, err := f.leaves(node.children, child, withLabel(labels, node.column, child.key))
				if err != nil {
					return nil, err
				}
				out = append(out, more...)
			}
			continue
		}
		values, err := metricValues(node, raw)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			name, leafLabels := v.name, labels
			if node.kind == aggPercentiles {
				leafLabels = withLabel(labels, "percentile", v.key)
			}
			out = append(out, leaf{labels: leafLabels, name: name, value: v.value})
		}
	}
	return out, nil
}

func (f *folder) addPoint(sb *seriesBuilder, l leaf, at time.Time) error {
	key := l.name + "\x00" + labelKey(l.labels)
	frame, ok := sb.frames[key]
	if !ok {
		if len(sb.keys) == f.lim.MaxSeries {
			return datasource.Errorf(datasource.ErrLimitExceeded, "series limit exceeded: more than %d series", f.lim.MaxSeries)
		}
		frame = &datasource.Frame{Name: l.name, Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{}},
			{Name: "Value", Type: datasource.FieldNumber, Values: []any{}},
		}}
		if len(l.labels) > 0 {
			frame.Labels = l.labels
		}
		sb.frames[key] = frame
		sb.keys = append(sb.keys, key)
	}
	if sb.points++; sb.points > f.lim.MaxPoints {
		return datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: more than %d points", f.lim.MaxPoints)
	}
	frame.Fields[0].Values = append(frame.Fields[0].Values, at)
	frame.Fields[1].Values = append(frame.Fields[1].Values, l.value)
	return nil
}

type tableBuilder struct {
	columns   []string
	rows      []map[string]any
	truncated bool
}

func (tb *tableBuilder) column(name string) {
	if !slices.Contains(tb.columns, name) {
		tb.columns = append(tb.columns, name)
	}
}

func (f *folder) table(nodes []aggNode, response map[string]json.RawMessage) ([]datasource.Frame, error) {
	tb := &tableBuilder{}
	root := bucket{docCount: float64(f.total), subs: response}
	if err := f.walkTable(tb, nodes, root, map[string]any{}); err != nil {
		return nil, err
	}
	fields := make([]datasource.Field, len(tb.columns))
	for i, name := range tb.columns {
		typ := datasource.FieldNumber
		if isBucketColumn(nodes, name) {
			typ = datasource.FieldString
		}
		fields[i] = datasource.Field{Name: name, Type: typ, Values: make([]any, 0, len(tb.rows))}
		for _, row := range tb.rows {
			fields[i].Values = append(fields[i].Values, row[name])
		}
	}
	frame := datasource.Frame{Fields: fields}
	if tb.truncated {
		frame.Meta = &datasource.FrameMeta{Truncated: true}
	}
	return []datasource.Frame{frame}, nil
}

func isBucketColumn(nodes []aggNode, name string) bool {
	for _, n := range nodes {
		if n.isBucket() {
			return n.column == name || isBucketColumn(n.children, name)
		}
	}
	return false
}

func (f *folder) walkTable(tb *tableBuilder, nodes []aggNode, b bucket, row map[string]any) error {
	idx := slices.IndexFunc(nodes, func(n aggNode) bool { return n.isBucket() })
	if idx >= 0 {
		node := nodes[idx]
		if len(nodes) > 1 {
			return unsupportedAgg(node.name, node.typ, "a metric beside a bucket aggregation in a table")
		}
		tb.column(node.column)
		raw, ok := b.subs[node.name]
		if !ok {
			return nil
		}
		buckets, err := parseBuckets(raw)
		if err != nil {
			return err
		}
		for _, child := range buckets {
			next := maps.Clone(row)
			next[node.column] = child.key
			if err := f.walkTable(tb, node.children, child, next); err != nil {
				return err
			}
		}
		return nil
	}
	if len(tb.rows) == f.lim.MaxRows {
		tb.truncated = true
		return nil
	}
	tb.column("count")
	row["count"] = datasource.Number(b.docCount)
	for _, node := range nodes {
		raw, ok := b.subs[node.name]
		if !ok {
			continue
		}
		values, err := metricValues(node, raw)
		if err != nil {
			return err
		}
		for _, v := range values {
			name := v.name
			if node.kind == aggPercentiles {
				name = v.name + " " + v.key
			}
			tb.column(name)
			row[name] = v.value
		}
	}
	tb.rows = append(tb.rows, row)
	return nil
}

func withLabel(labels map[string]string, name, value string) map[string]string {
	out := make(map[string]string, len(labels))
	maps.Copy(out, labels)
	out[name] = value
	return out
}

func labelKey(labels map[string]string) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		fmt.Fprintf(&b, "%s=%q,", k, labels[k])
	}
	return b.String()
}
