package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	MaxAggRows     = 100000
	maxBuckets     = 11000
	maxAggNodes    = 50
	maxTermsSize   = 1000
	defaultSize    = 10
	maxLevels      = 3
	maxPercents    = 20
	maxInterval    = 366 * 24 * time.Hour
	supportedAggs  = "date_histogram, terms, cardinality, value_count, count, min, max, avg, sum and percentiles"
	orderCountDesc = "count_desc"
	orderCountAsc  = "count_asc"
	orderKeyAsc    = "key_asc"
	orderKeyDesc   = "key_desc"
)

var (
	defaultPercents   = []float64{1, 5, 25, 50, 75, 95, 99}
	intervalPattern   = regexp.MustCompile(`^([1-9][0-9]*)(ms|s|m|h|d)$`)
	calendarIntervals = map[string]string{"minute": "1m", "1m": "1m", "hour": "1h", "1h": "1h", "day": "1d", "1d": "1d"}
	intervalUnits     = map[string]time.Duration{"ms": time.Millisecond, "s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}
)

type Agg struct {
	Name     string
	Type     string
	Field    string
	Interval time.Duration
	Size     int
	Order    string
	Percents []float64
	Aggs     []Agg
}

func (a Agg) isBucket() bool {
	return a.Type == "date_histogram" || a.Type == "terms"
}

type Level struct {
	Name  string
	Type  string
	Field string
}

type Metric struct {
	Name string
	Type string
	Key  string
}

type Aggregation struct {
	SQL      string
	Args     []any
	Levels   []Level
	Metrics  []Metric
	RowLimit int
}

func (a Aggregation) HasTimeBucket() bool {
	return slices.ContainsFunc(a.Levels, func(l Level) bool { return l.Type == "date_histogram" })
}

type aggParser struct {
	nodes int
}

func parseAggs(tree map[string]any) ([]Agg, error) {
	p := &aggParser{}
	aggs, err := p.level(tree)
	if err != nil {
		return nil, err
	}
	if _, err := planAggs(aggs); err != nil {
		return nil, err
	}
	return aggs, nil
}

func (p *aggParser) level(tree map[string]any) ([]Agg, error) {
	out := make([]Agg, 0, len(tree))
	for _, name := range slices.Sorted(maps.Keys(tree)) {
		if p.nodes++; p.nodes > maxAggNodes {
			return nil, failOn(name, "too many aggregations; at most %d", maxAggNodes)
		}
		a, err := p.agg(name, tree[name])
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (p *aggParser) agg(name string, raw any) (Agg, error) {
	def, ok := raw.(map[string]any)
	if !ok {
		return Agg{}, failOn(name, "aggregation %q must be an object", truncate(name))
	}
	a := Agg{Name: name}
	var sub map[string]any
	for _, key := range slices.Sorted(maps.Keys(def)) {
		switch key {
		case "aggs", "aggregations":
			sub, _ = def[key].(map[string]any)
		case "meta":
		default:
			if a.Type != "" {
				return Agg{}, failOn(name, "aggregation %q has more than one type", truncate(name))
			}
			a.Type = key
		}
	}
	if err := a.params(def[a.Type]); err != nil {
		return Agg{}, err
	}
	if len(sub) == 0 {
		return a, nil
	}
	if !a.isBucket() {
		return Agg{}, failOn(name, "%s aggregation %q takes no sub-aggregations", a.Type, truncate(name))
	}
	children, err := p.level(sub)
	a.Aggs = children
	return a, err
}

func (a *Agg) params(raw any) error {
	obj, ok := raw.(map[string]any)
	if !ok {
		return failOn(a.Name, "unsupported aggregation %q (type %s); supported: %s", truncate(a.Name), truncate(a.Type), supportedAggs)
	}
	switch a.Type {
	case "date_histogram":
		return a.dateHistogram(obj)
	case "terms":
		return a.terms(obj)
	case "cardinality":
		return a.fieldMetric(obj, "field", "precision_threshold")
	case "value_count", "count":
		return a.fieldMetric(obj, "field")
	case "min", "max", "avg", "sum":
		return a.numericMetric(obj, "field")
	case "percentiles":
		return a.percentiles(obj)
	}
	return failOn(a.Name, "unsupported aggregation %q (type %s); supported: %s", truncate(a.Name), truncate(a.Type), supportedAggs)
}

func (a *Agg) dateHistogram(obj map[string]any) error {
	if err := onlyKeys(obj, "date_histogram "+a.Name, "field", "fixed_interval", "calendar_interval", "min_doc_count", "extended_bounds", "time_zone", "format"); err != nil {
		return err
	}
	if f, ok := obj["field"].(string); ok && f != occurredField {
		return failOn(a.Name, "date_histogram can only bucket %s, not %q", occurredField, truncate(f))
	}
	if zone, ok := obj["time_zone"].(string); ok && !strings.EqualFold(zone, "UTC") {
		return failOn(a.Name, "date_histogram supports only the UTC time zone")
	}
	text, _ := obj["fixed_interval"].(string)
	if calendar, ok := obj["calendar_interval"].(string); ok && text == "" {
		text = calendarIntervals[calendar]
	}
	m := intervalPattern.FindStringSubmatch(text)
	if m == nil {
		return failOn(a.Name, "date_histogram %q needs a fixed_interval such as 30s, 5m, 1h or 1d, or a calendar_interval of minute, hour or day", truncate(a.Name))
	}
	unit := intervalUnits[m[2]]
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n > int64(maxInterval/unit) {
		return failOn(a.Name, "date_histogram interval is longer than 366 days")
	}
	a.Field = occurredField
	a.Interval = time.Duration(n) * unit
	return nil
}

func (a *Agg) terms(obj map[string]any) error {
	if err := onlyKeys(obj, "terms "+a.Name, "field", "size", "order", "min_doc_count"); err != nil {
		return err
	}
	f, err := a.groupField(obj)
	if err != nil {
		return err
	}
	a.Field, a.Size, a.Order = f.Name, defaultSize, orderCountDesc
	if v, ok := obj["size"]; ok {
		n, ok := wholeNumber(v)
		if !ok || n < 1 || n > maxTermsSize {
			return failOn(a.Name, "terms size must be a whole number from 1 to %d", maxTermsSize)
		}
		a.Size = n
	}
	if v, ok := obj["min_doc_count"]; ok {
		if n, ok := wholeNumber(v); !ok || n != 1 {
			return failOn(a.Name, "terms supports only min_doc_count 1")
		}
	}
	if v, ok := obj["order"]; ok {
		if a.Order, err = termsOrder(a.Name, v); err != nil {
			return err
		}
	}
	return nil
}

func termsOrder(name string, v any) (string, error) {
	if list, ok := v.([]any); ok && len(list) == 1 {
		v = list[0]
	}
	obj, ok := v.(map[string]any)
	if !ok || len(obj) != 1 {
		return "", failOn(name, "terms order must be one of _count or _key with asc or desc")
	}
	for key, dir := range obj {
		switch key + ":" + fmt.Sprint(dir) {
		case "_count:desc":
			return orderCountDesc, nil
		case "_count:asc":
			return orderCountAsc, nil
		case "_key:asc":
			return orderKeyAsc, nil
		case "_key:desc":
			return orderKeyDesc, nil
		}
	}
	return "", failOn(name, "terms order must be one of _count or _key with asc or desc")
}

func (a *Agg) groupField(obj map[string]any) (*Field, error) {
	name, _ := obj["field"].(string)
	f, ok := lookupField(name)
	if ok && !f.groupable() {
		return nil, failOn(a.Name, "%s %q needs a field with values to group by, not %s", a.Type, truncate(a.Name), name)
	}
	if !ok {
		return nil, failOn(a.Name, "%s %q needs a field: %s", a.Type, truncate(a.Name), unknownFieldMessage(name))
	}
	return f, nil
}

func (a *Agg) fieldMetric(obj map[string]any, allowed ...string) error {
	if err := onlyKeys(obj, a.Type+" "+a.Name, allowed...); err != nil {
		return err
	}
	if _, ok := obj["field"]; !ok && a.Type == "count" {
		return nil
	}
	f, err := a.groupField(obj)
	if err != nil {
		return err
	}
	a.Field = f.Name
	return nil
}

func (a *Agg) numericMetric(obj map[string]any, allowed ...string) error {
	if err := a.fieldMetric(obj, allowed...); err != nil {
		return err
	}
	if f := fieldIndex[a.Field]; f.Type != TypeInt {
		return failOn(a.Name, "%s needs a numeric field such as %s, not %s", a.Type, durationField, a.Field)
	}
	return nil
}

func (a *Agg) percentiles(obj map[string]any) error {
	if err := a.numericMetric(withoutKey(obj, "percents", "keyed"), "field"); err != nil {
		return err
	}
	if err := onlyKeys(obj, "percentiles "+a.Name, "field", "percents", "keyed"); err != nil {
		return err
	}
	a.Percents = defaultPercents
	raw, ok := obj["percents"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 || len(list) > maxPercents {
		return failOn(a.Name, "percents must be a list of 1 to %d numbers between 0 and 100", maxPercents)
	}
	a.Percents = make([]float64, len(list))
	for i, item := range list {
		n, ok := item.(json.Number)
		v, err := n.Float64()
		if !ok || err != nil || math.IsNaN(v) || v < 0 || v > 100 {
			return failOn(a.Name, "percents must be a list of 1 to %d numbers between 0 and 100", maxPercents)
		}
		a.Percents[i] = v
	}
	return nil
}

func withoutKey(obj map[string]any, keys ...string) map[string]any {
	out := maps.Clone(obj)
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

func wholeNumber(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(n.String())
	return i, err == nil
}

type aggPlan struct {
	levels  []Agg
	metrics []Agg
	groups  int
}

func planAggs(aggs []Agg) (aggPlan, error) {
	var plan aggPlan
	timeBuckets, termsLevels, product := 0, 0, 1
	for current := aggs; len(current) > 0; {
		var bucket *Agg
		var metrics []Agg
		for i := range current {
			if !current[i].isBucket() {
				metrics = append(metrics, current[i])
				continue
			}
			if bucket != nil {
				return plan, failOn(current[i].Name, "one bucket aggregation per level; %q is the second", truncate(current[i].Name))
			}
			bucket = &current[i]
		}
		if bucket == nil {
			plan.metrics = metrics
			break
		}
		if len(metrics) > 0 {
			return plan, failOn(metrics[0].Name, "put the metric %q inside the innermost bucket aggregation", truncate(metrics[0].Name))
		}
		if bucket.Type == "date_histogram" {
			timeBuckets++
		} else {
			termsLevels++
			product *= bucket.Size
		}
		plan.levels = append(plan.levels, *bucket)
		current = bucket.Aggs
	}
	switch {
	case len(plan.levels) > maxLevels:
		return plan, errors.New("aggregations nest at most three levels")
	case timeBuckets > 1:
		return plan, errors.New("only one date_histogram is supported")
	case termsLevels > 2:
		return plan, errors.New("at most two terms levels are supported")
	case product > maxTermsSize:
		return plan, fmt.Errorf("the terms sizes multiply to more than %d groups", maxTermsSize)
	}
	plan.groups = product
	return plan, nil
}

func (q Query) Aggregate(w Window) (Aggregation, error) {
	if len(q.Aggs) == 0 {
		return Aggregation{}, errors.New("the query has no aggs; use a Query DSL body with aggs")
	}
	return q.aggregate(w, q.Aggs)
}

func (q Query) Histogram(w Window, step time.Duration) (Aggregation, error) {
	if step < time.Millisecond {
		return Aggregation{}, ErrStep
	}
	return q.aggregate(w, []Agg{{Name: "count", Type: "date_histogram", Field: occurredField, Interval: step}})
}

func (q Query) aggregate(w Window, aggs []Agg) (Aggregation, error) {
	plan, err := planAggs(aggs)
	if err != nil {
		return Aggregation{}, err
	}
	for _, l := range plan.levels {
		if l.Type != "date_histogram" {
			continue
		}
		buckets := int64(w.To.Sub(w.From)/l.Interval) + 1
		if buckets > maxBuckets {
			return Aggregation{}, fmt.Errorf("interval too small: more than %d buckets in the window", maxBuckets)
		}
		if buckets*int64(plan.groups) > MaxAggRows {
			return Aggregation{}, fmt.Errorf("the date_histogram buckets and terms sizes multiply to more than %d rows; use a larger interval, a smaller size or a shorter window", MaxAggRows)
		}
	}
	b := &builder{}
	where, err := b.where(q, w)
	if err != nil {
		return Aggregation{}, err
	}
	out := Aggregation{RowLimit: MaxAggRows + 1}
	keys := keyNames(len(plan.levels))
	selects := make([]string, 0, len(plan.levels)+len(plan.metrics))
	for i, l := range plan.levels {
		expr, filter := levelExpr(b, l)
		selects = append(selects, expr+" AS "+keys[i])
		if filter != "" {
			where += " AND " + filter
		}
		out.Levels = append(out.Levels, Level{Name: l.Name, Type: l.Type, Field: l.Field})
	}
	metricSQL, inputs := metricColumns(b, plan.metrics, &out)
	selects = append(selects, inputs...)
	if len(selects) == 0 {
		selects = []string{"1 AS one"}
	}
	ctes := []string{"matched AS (SELECT " + strings.Join(selects, ", ") + " FROM " + tableName + " WHERE " + where + ")"}
	last, lastKeys, ranked := "", []string(nil), 0
	for i, l := range plan.levels {
		if l.Type != "terms" {
			continue
		}
		ranked++
		cte := "t" + strconv.Itoa(ranked)
		ctes = append(ctes, cte+" AS ("+topTerms(b, l, keys[:i], keys[i], last, lastKeys)+")")
		lastKeys = keys[:i+1]
		last = cte
	}
	out.SQL = finalSQL(b, ctes, keys, lastKeys, last, metricSQL)
	out.Args = b.args
	return out, nil
}

func keyNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = "k" + strconv.Itoa(i+1)
	}
	return names
}

func levelExpr(b *builder, l Agg) (expr, filter string) {
	if l.Type == "date_histogram" {
		secs := b.add(l.Interval.Seconds())
		origin := b.add(time.Unix(0, 0).UTC())
		return "date_bin(make_interval(secs => " + secs + "::double precision), occurred_at, " + origin + "::timestamptz)", ""
	}
	f := fieldIndex[l.Field]
	return f.textExpr(), f.column + " IS NOT NULL"
}

func metricColumns(b *builder, metrics []Agg, out *Aggregation) (exprs, inputs []string) {
	for i, m := range metrics {
		input := "m" + strconv.Itoa(i+1)
		over := "matched." + input
		if m.Field != "" {
			inputs = append(inputs, fieldIndex[m.Field].column+" AS "+input)
		}
		switch m.Type {
		case "cardinality":
			exprs = append(exprs, "count(DISTINCT "+over+")::double precision")
		case "value_count", "count":
			exprs = append(exprs, countExpr(m, over))
		case "percentiles":
			for _, p := range m.Percents {
				exprs = append(exprs, "percentile_cont("+b.add(p/100)+"::double precision) WITHIN GROUP (ORDER BY "+over+")")
				out.Metrics = append(out.Metrics, Metric{Name: m.Name, Type: m.Type, Key: strconv.FormatFloat(p, 'f', -1, 64)})
			}
			continue
		default:
			exprs = append(exprs, m.Type+"("+over+")::double precision")
		}
		out.Metrics = append(out.Metrics, Metric{Name: m.Name, Type: m.Type})
	}
	return exprs, inputs
}

func countExpr(m Agg, over string) string {
	if m.Field == "" {
		return "count(*)::double precision"
	}
	return "count(" + over + ")::double precision"
}

func topTerms(b *builder, l Agg, parents []string, key, previous string, previousKeys []string) string {
	partition, from := "", "matched"
	cols := make([]string, 0, len(parents)+1)
	for _, p := range parents {
		cols = append(cols, "matched."+p)
	}
	if len(parents) > 0 {
		partition = "PARTITION BY " + strings.Join(cols, ", ") + " "
	}
	if previous != "" {
		from += " JOIN " + previous + " ON " + joinOn("matched", previous, previousKeys)
	}
	cols = append(cols, "matched."+key)
	order := map[string]string{
		orderCountDesc: "count(*) DESC, matched." + key,
		orderCountAsc:  "count(*) ASC, matched." + key,
		orderKeyAsc:    "matched." + key + " ASC",
		orderKeyDesc:   "matched." + key + " DESC",
	}[l.Order]
	names := append(slices.Clone(parents), key)
	return "SELECT " + strings.Join(names, ", ") + " FROM (SELECT " + strings.Join(cols, ", ") +
		", row_number() OVER (" + partition + "ORDER BY " + order + ") AS rn FROM " + from +
		" GROUP BY " + strings.Join(cols, ", ") + ") ranked WHERE rn <= " + b.add(l.Size)
}

func joinOn(left, right string, keys []string) string {
	conds := make([]string, len(keys))
	for i, k := range keys {
		conds[i] = left + "." + k + " = " + right + "." + k
	}
	return strings.Join(conds, " AND ")
}

func finalSQL(b *builder, ctes, keys, termKeys []string, last string, metricSQL []string) string {
	selects := make([]string, 0, len(keys)+1+len(metricSQL))
	ordinals := make([]string, len(keys))
	for i, k := range keys {
		selects = append(selects, "matched."+k)
		ordinals[i] = strconv.Itoa(i + 1)
	}
	selects = append(selects, "count(*)::double precision AS doc_count")
	selects = append(selects, metricSQL...)
	sql := "WITH " + strings.Join(ctes, ", ") + " SELECT " + strings.Join(selects, ", ") + " FROM matched"
	if last != "" {
		sql += " JOIN " + last + " ON " + joinOn("matched", last, termKeys)
	}
	if len(ordinals) > 0 {
		list := strings.Join(ordinals, ", ")
		sql += " GROUP BY " + list + " ORDER BY " + list
	}
	return sql + " LIMIT " + b.add(MaxAggRows+1)
}
