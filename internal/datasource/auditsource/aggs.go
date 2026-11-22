package auditsource

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
)

const (
	maxFilledBuckets = 11000
	maxAggregations  = 20
	maxTermsSize     = 1000
	dateHistogram    = "date_histogram"
	terms            = "terms"
	keyFormat        = "2006-01-02T15:04:05.000Z"
)

var (
	epoch        = time.Unix(0, 0).UTC()
	countMetrics = []string{"value_count", "cardinality", "count"}
	numericKeys  = numericFields()
)

func numericFields() map[string]bool {
	out := map[string]bool{}
	for _, f := range filter.Fields() {
		if f.Type == filter.TypeInt {
			out[f.Name] = true
		}
	}
	return out
}

type bucketBudget struct {
	left int
}

func (b *bucketBudget) spend(n int) error {
	b.left -= n
	if b.left < 0 {
		return refuse("the aggregations fill more than %d buckets in one request; use a larger interval or a shorter window", maxFilledBuckets)
	}
	return nil
}

func countAggs(tree map[string]any) int {
	n := 0
	for _, v := range tree {
		n++
		if def, ok := v.(map[string]any); ok {
			for _, key := range []string{"aggs", "aggregations"} {
				if sub, ok := def[key].(map[string]any); ok {
					n += countAggs(sub)
				}
			}
		}
	}
	return n
}

func (e *emulator) aggregate(ctx context.Context, clause map[string]any, w filter.Window, tree map[string]any) (map[string]any, error) {
	if countAggs(tree) > maxAggregations {
		return nil, refuse("a request may hold at most %d aggregations, nested ones included", maxAggregations)
	}
	budget := &bucketBudget{left: maxFilledBuckets}
	out := map[string]any{}
	metrics := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(tree)) {
		def, ok := tree[name].(map[string]any)
		if !ok {
			return nil, refuse("aggregation %q must be an object", name)
		}
		if !isBucketDef(def) {
			metrics[name] = def
			continue
		}
		rendered, err := e.bucketGroup(ctx, clause, w, name, def, budget)
		if err != nil {
			return nil, err
		}
		out[name] = rendered
	}
	if len(metrics) > 0 {
		q, err := parseAggs(clause, metrics)
		if err != nil {
			return nil, err
		}
		agg, err := e.store.ReadAggregation(ctx, trail.Prepared{Query: q, Window: w})
		if err != nil {
			return nil, err
		}
		var values []*float64
		if len(agg.Rows) > 0 {
			values = agg.Rows[0].Values
		}
		maps.Copy(out, metricsOf(agg.Metrics, values))
	}
	return out, nil
}

func isBucketDef(def map[string]any) bool {
	for _, key := range []string{"aggs", "aggregations", dateHistogram, terms} {
		if _, ok := def[key]; ok {
			return true
		}
	}
	return false
}

func parseAggs(clause map[string]any, tree map[string]any) (filter.Query, error) {
	raw, err := json.Marshal(map[string]any{"aggs": tree})
	if err != nil {
		return filter.Query{}, refuse("the aggregation cannot be encoded")
	}
	q, err := filter.Parse(string(raw))
	if err != nil {
		return filter.Query{}, err
	}
	q.Clause = clause
	return q, nil
}

type level struct {
	agg      filter.Agg
	def      map[string]any
	asked    int
	extraOne bool
}

func subAggs(def map[string]any) map[string]any {
	for _, key := range []string{"aggs", "aggregations"} {
		if sub, ok := def[key].(map[string]any); ok {
			return sub
		}
	}
	return nil
}

func bucketChild(def map[string]any) map[string]any {
	sub := subAggs(def)
	for _, name := range slices.Sorted(maps.Keys(sub)) {
		if child, ok := sub[name].(map[string]any); ok && isBucketDef(child) {
			return child
		}
	}
	return nil
}

func termsBody(def map[string]any) map[string]any {
	body, _ := def[terms].(map[string]any)
	return body
}

func sizeTerms(def map[string]any) ([]map[string]any, error) {
	var termsDefs []map[string]any
	for at := def; at != nil; at = bucketChild(at) {
		if body := termsBody(at); body != nil {
			termsDefs = append(termsDefs, body)
		}
	}
	for _, body := range termsDefs {
		size := 10
		if v, ok := body["size"]; ok {
			n, ok := wholeNumber(v)
			if !ok || n < 1 {
				return nil, refuse("terms size must be a whole number of 1 or more")
			}
			size = n
		}
		body["size"] = json.Number(strconv.Itoa(min(size, maxTermsSize)))
	}
	return termsDefs, nil
}

func deepCopy(def map[string]any) map[string]any {
	raw, _ := json.Marshal(def)
	out, _ := decodeBody(raw)
	return out
}

func (e *emulator) bucketGroup(ctx context.Context, clause map[string]any, w filter.Window, name string, def map[string]any, budget *bucketBudget) (map[string]any, error) {
	def = deepCopy(def)
	termsDefs, err := sizeTerms(def)
	if err != nil {
		return nil, err
	}
	extra := len(termsDefs) == 1
	var askedSize int
	if extra {
		askedSize, _ = wholeNumber(termsDefs[0]["size"])
		extra = askedSize < maxTermsSize
		if extra {
			termsDefs[0]["size"] = json.Number(strconv.Itoa(askedSize + 1))
		}
	}
	q, err := parseAggs(clause, map[string]any{name: def})
	if err != nil {
		return nil, err
	}
	var chain []level
	rawAt := def
	for aggs := q.Aggs; len(aggs) > 0; {
		i := slices.IndexFunc(aggs, func(a filter.Agg) bool { return a.Type == dateHistogram || a.Type == terms })
		if i < 0 {
			break
		}
		l := level{agg: aggs[i], def: rawAt}
		if l.agg.Type == terms {
			l.asked = l.agg.Size
			if extra {
				l.asked, l.extraOne = askedSize, true
			}
		}
		chain = append(chain, l)
		aggs = aggs[i].Aggs
		rawAt = bucketChild(rawAt)
	}
	results := make([]trail.Aggregation, len(chain))
	for depth := range chain {
		prefix := q
		if depth < len(chain)-1 {
			prefix = filter.Query{Clause: clause, Aggs: []filter.Agg{prefixAgg(chain, depth)}}
		}
		agg, err := e.store.ReadAggregation(ctx, trail.Prepared{Query: prefix, Window: w})
		if err != nil {
			return nil, err
		}
		if agg.Truncated {
			return nil, tooManyBuckets("the aggregation has more than %d rows; use a larger interval, a smaller size or a shorter window", filter.MaxAggRows)
		}
		results[depth] = agg
	}
	roots := treeOf(results)
	return renderLevel(chain, 0, roots, results[len(results)-1].Metrics, budget)
}

func prefixAgg(chain []level, depth int) filter.Agg {
	a := chain[depth].agg
	a.Aggs = nil
	for i := depth - 1; i >= 0; i-- {
		parent := chain[i].agg
		parent.Aggs = []filter.Agg{a}
		a = parent
	}
	return a
}

type bucketNode struct {
	key      any
	docCount int64
	values   []*float64
	children []*bucketNode
}

func pathKey(keys []any) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i], _ = k.(string)
	}
	return strings.Join(parts, "\x00")
}

func treeOf(results []trail.Aggregation) []*bucketNode {
	var roots []*bucketNode
	byPath := map[string]*bucketNode{}
	for depth, agg := range results {
		for _, row := range agg.Rows {
			n := &bucketNode{key: row.Keys[depth], docCount: row.DocCount, values: row.Values}
			byPath[pathKey(row.Keys)] = n
			if depth == 0 {
				roots = append(roots, n)
				continue
			}
			if parent, ok := byPath[pathKey(row.Keys[:depth])]; ok {
				parent.children = append(parent.children, n)
			}
		}
	}
	return roots
}

func renderLevel(chain []level, depth int, nodes []*bucketNode, metrics []filter.Metric, budget *bucketBudget) (map[string]any, error) {
	l := chain[depth]
	render := func(n *bucketNode, b map[string]any) error {
		b["doc_count"] = n.docCount
		if depth == len(chain)-1 {
			maps.Copy(b, metricsOf(metrics, n.values))
			return nil
		}
		child, err := renderLevel(chain, depth+1, n.children, metrics, budget)
		if err != nil {
			return err
		}
		b[chain[depth+1].agg.Name] = child
		return nil
	}
	if l.agg.Type == dateHistogram {
		filled, err := fillTimes(l, nodes)
		if err != nil {
			return nil, err
		}
		if err := budget.spend(len(filled)); err != nil {
			return nil, err
		}
		buckets := make([]any, 0, len(filled))
		for _, n := range filled {
			at := n.key.(time.Time)
			b := map[string]any{"key": at.UnixMilli(), "key_as_string": at.UTC().Format(keyFormat)}
			if err := render(n, b); err != nil {
				return nil, err
			}
			buckets = append(buckets, b)
		}
		return map[string]any{"buckets": buckets}, nil
	}
	sortTerms(nodes, l.agg.Order)
	var other int64
	if l.extraOne && len(nodes) > l.asked {
		for _, n := range nodes[l.asked:] {
			other += n.docCount
		}
		nodes = nodes[:l.asked]
	}
	buckets := make([]any, 0, len(nodes))
	for _, n := range nodes {
		b := map[string]any{"key": termKey(l.agg.Field, n.key)}
		if err := render(n, b); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
	}
	return map[string]any{"doc_count_error_upper_bound": 0, "sum_other_doc_count": other, "buckets": buckets}, nil
}

func termKey(field string, key any) any {
	text, _ := key.(string)
	if numericKeys[field] {
		if _, err := strconv.ParseInt(text, 10, 64); err == nil {
			return json.Number(text)
		}
	}
	return text
}

func compareKeys(a, b any) int {
	x, _ := a.(string)
	y, _ := b.(string)
	xi, errX := strconv.ParseInt(x, 10, 64)
	yi, errY := strconv.ParseInt(y, 10, 64)
	if errX == nil && errY == nil {
		return cmp.Compare(xi, yi)
	}
	return strings.Compare(x, y)
}

func sortTerms(nodes []*bucketNode, order string) {
	slices.SortStableFunc(nodes, func(a, b *bucketNode) int {
		switch order {
		case "count_asc":
			return cmp.Or(cmp.Compare(a.docCount, b.docCount), compareKeys(a.key, b.key))
		case "key_asc":
			return compareKeys(a.key, b.key)
		case "key_desc":
			return compareKeys(b.key, a.key)
		}
		return cmp.Or(cmp.Compare(b.docCount, a.docCount), compareKeys(a.key, b.key))
	})
}

func fillTimes(l level, nodes []*bucketNode) ([]*bucketNode, error) {
	byMS := map[int64]*bucketNode{}
	for _, n := range nodes {
		text, _ := n.key.(string)
		at, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, refuse("the aggregation returned an unreadable time bucket")
		}
		n.key = at.UTC()
		byMS[at.UnixMilli()] = n
	}
	if fillsEmpty(l.def) {
		loc := time.UTC
		if l.agg.Zone != "" {
			var err error
			if loc, err = filter.NamedZone(l.agg.Zone); err != nil {
				return nil, refuse("date_histogram time_zone %q is not a named zone", l.agg.Zone)
			}
		}
		low, high, ok, err := fillBounds(l.def, nodes)
		if err != nil {
			return nil, err
		}
		if ok {
			grid, err := binsBetween(low, high, l.agg.Interval, loc)
			if err != nil {
				return nil, err
			}
			for _, at := range grid {
				if _, present := byMS[at.UnixMilli()]; !present {
					byMS[at.UnixMilli()] = &bucketNode{key: at}
				}
			}
		}
	}
	out := slices.Collect(maps.Values(byMS))
	slices.SortFunc(out, func(a, b *bucketNode) int { return a.key.(time.Time).Compare(b.key.(time.Time)) })
	return out, nil
}

func fillsEmpty(def map[string]any) bool {
	body, _ := def[dateHistogram].(map[string]any)
	v, ok := body["min_doc_count"]
	if !ok {
		return true
	}
	n, isWhole := wholeNumber(v)
	return isWhole && n == 0
}

func fillBounds(def map[string]any, nodes []*bucketNode) (time.Time, time.Time, bool, error) {
	body, _ := def[dateHistogram].(map[string]any)
	if bounds, ok := body["extended_bounds"].(map[string]any); ok {
		low, err := boundTime(bounds["min"])
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		high, err := boundTime(bounds["max"])
		return low, high, err == nil, err
	}
	if len(nodes) == 0 {
		return time.Time{}, time.Time{}, false, nil
	}
	low, high := nodes[0].key.(time.Time), nodes[0].key.(time.Time)
	for _, n := range nodes[1:] {
		at := n.key.(time.Time)
		low, high = minTime(low, at), maxTime(high, at)
	}
	return low, high, true, nil
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func boundTime(v any) (time.Time, error) {
	switch x := v.(type) {
	case json.Number:
		if ms, err := strconv.ParseInt(x.String(), 10, 64); err == nil {
			return time.UnixMilli(ms).UTC(), nil
		}
	case string:
		if at, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return at.UTC(), nil
		}
	}
	return time.Time{}, refuse("extended_bounds min and max must be epoch milliseconds or RFC 3339 times")
}

func naive(t time.Time, loc *time.Location) time.Time {
	w := t.In(loc)
	return time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), time.UTC)
}

func located(n time.Time, loc *time.Location) time.Time {
	return time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), n.Nanosecond(), loc).UTC()
}

func binStart(t time.Time, step time.Duration, loc *time.Location) time.Time {
	n := naive(t, loc)
	return located(epoch.Add(n.Sub(epoch).Truncate(step)), loc)
}

func binsBetween(low, high time.Time, step time.Duration, loc *time.Location) ([]time.Time, error) {
	if step <= 0 || high.Before(low) {
		return nil, nil
	}
	if int64(high.Sub(low)/step) >= maxFilledBuckets {
		return nil, tooManyBuckets("more than %d buckets between the date_histogram bounds", maxFilledBuckets)
	}
	last := binStart(high, step, loc)
	var out []time.Time
	for n := naive(binStart(low, step, loc), loc); ; n = n.Add(step) {
		at := located(n, loc)
		if at.After(last) {
			return out, nil
		}
		if len(out) == 0 || at.After(out[len(out)-1]) {
			out = append(out, at)
		}
	}
}

func metricsOf(metrics []filter.Metric, values []*float64) map[string]any {
	out := map[string]any{}
	for i, m := range metrics {
		var v *float64
		if i < len(values) {
			v = values[i]
		}
		if v == nil && slices.Contains(countMetrics, m.Type) {
			zero := 0.0
			v = &zero
		}
		if m.Type != "percentiles" {
			out[m.Name] = map[string]any{"value": v}
			continue
		}
		entry, _ := out[m.Name].(map[string]any)
		if entry == nil {
			entry = map[string]any{"values": map[string]any{}}
			out[m.Name] = entry
		}
		entry["values"].(map[string]any)[percentKey(m.Key)] = v
	}
	return out
}

func percentKey(key string) string {
	if strings.Contains(key, ".") {
		return key
	}
	return key + ".0"
}
