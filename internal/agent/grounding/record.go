package grounding

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	MaxValues       = 2000
	maxValuesOfRead = 500
	maxNames        = 400
	maxNameBytes    = 64
	maxRanges       = 64
	maxInstants     = 256
	maxScannedBytes = 4 << 10
	sigFigs         = 3
)

var (
	identifier  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:/-]*$`)
	quotedValue = regexp.MustCompile(`=\s*"([^"]{1,64})"`)
)

type Range struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type Record struct {
	Mode     Mode        `json:"mode"`
	Reads    [][]float64 `json:"reads,omitempty"`
	Names    []string    `json:"names,omitempty"`
	Ranges   []Range     `json:"ranges,omitempty"`
	Instants []time.Time `json:"instants,omitempty"`
}

func NewRecord(mode Mode) *Record {
	if mode == Off || mode == "" {
		return nil
	}
	return &Record{Mode: mode}
}

func (r *Record) Enforced() bool {
	return r != nil && r.Mode == Enforce
}

func (r *Record) AddRead(content []byte, names []string, ranges []Range) {
	if r == nil {
		return
	}
	read := reading{record: r, seen: map[float64]bool{}}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var parsed any
	if decoder.Decode(&parsed) == nil {
		read.walk(parsed, slot{})
	}
	if len(read.values) > 0 {
		r.Reads = append(r.Reads, read.values)
		r.keepValues()
	}
	for _, name := range names {
		r.addName(name)
	}
	for _, rng := range ranges {
		r.addRange(rng)
	}
}

func (r *Record) keepValues() {
	total := 0
	for _, read := range r.Reads {
		total += len(read)
	}
	for over := total - MaxValues; over > 0; {
		if oldest := len(r.Reads[0]); oldest <= over {
			r.Reads, over = r.Reads[1:], over-oldest
			continue
		}
		r.Reads[0], over = r.Reads[0][over:], 0
	}
}

type slot struct {
	key  string
	time bool
}

type reading struct {
	record *Record
	seen   map[float64]bool
	values []float64
}

func (rd *reading) walk(v any, at slot) {
	switch v := v.(type) {
	case map[string]any:
		if rd.table(v) {
			return
		}
		for _, key := range sortedKeys(v) {
			rd.walk(v[key], slot{key: key, time: timeKey(key)})
		}
	case []any:
		for _, child := range v {
			rd.walk(child, at)
		}
	case json.Number:
		if f, err := strconv.ParseFloat(string(v), 64); err == nil && !at.time {
			rd.value(f)
		}
	case string:
		rd.text(v, at)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (rd *reading) table(m map[string]any) bool {
	columns, ok := m["columns"].([]any)
	rows, hasRows := m["rows"].([]any)
	if !ok || !hasRows {
		return false
	}
	cells := make([]slot, len(columns))
	for i, column := range columns {
		switch c := column.(type) {
		case map[string]any:
			name, _ := c["name"].(string)
			kind, _ := c["type"].(string)
			cells[i] = slot{key: name, time: kind == "time" || timeKey(name)}
		case string:
			cells[i] = slot{key: c, time: timeKey(c)}
		}
	}
	for _, row := range rows {
		values, _ := row.([]any)
		for i, cell := range values {
			if i < len(cells) {
				rd.walk(cell, cells[i])
			}
		}
	}
	return true
}

var timeKeys = map[string]bool{
	"time": true, "timens": true, "timestamp": true, "ts": true, "t": true,
	"start": true, "end": true, "from": true, "to": true, "at": true,
	"startns": true, "endns": true, "starttime": true, "endtime": true,
	"timeunixnano": true, "startunixnano": true, "endunixnano": true,
}

func timeKey(key string) bool {
	return timeKeys[normalName(key)]
}

func (rd *reading) text(s string, at slot) {
	s = cut(s)
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		if !at.time {
			rd.value(f)
		}
		return
	}
	if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s)); err == nil {
		rd.record.addInstant(t)
		return
	}
	if identifier.MatchString(s) {
		rd.record.addName(s)
	}
	for _, m := range quotedValue.FindAllStringSubmatch(s, -1) {
		rd.record.addName(m[1])
	}
	for _, f := range valuesIn(s) {
		rd.value(f)
	}
}

func cut(s string) string {
	if len(s) <= maxScannedBytes {
		return s
	}
	s = s[:maxScannedBytes]
	if i := strings.LastIndexFunc(s, unicode.IsSpace); i > 0 {
		return s[:i]
	}
	return s
}

func (rd *reading) value(v float64) {
	v = rounded(v)
	if math.IsNaN(v) || math.IsInf(v, 0) || rd.seen[v] || len(rd.values) >= maxValuesOfRead {
		return
	}
	rd.seen[v] = true
	rd.values = append(rd.values, v)
}

func rounded(v float64) float64 {
	v = math.Abs(v)
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	out, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', sigFigs, 64), 64)
	if err != nil {
		return v
	}
	return out
}

func (r *Record) addName(name string) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameBytes {
		return
	}
	if i := slices.Index(r.Names, name); i >= 0 {
		r.Names = slices.Delete(r.Names, i, i+1)
	}
	r.Names = append(r.Names, name)
	if over := len(r.Names) - maxNames; over > 0 {
		r.Names = slices.Delete(r.Names, 0, over)
	}
}

func (r *Record) addRange(rng Range) {
	if !rng.From.Before(rng.To) {
		return
	}
	rng = Range{From: rng.From.UTC(), To: rng.To.UTC()}
	if slices.Contains(r.Ranges, rng) {
		return
	}
	r.Ranges = append(r.Ranges, rng)
	if over := len(r.Ranges) - maxRanges; over > 0 {
		r.Ranges = slices.Delete(r.Ranges, 0, over)
	}
}

func (r *Record) addInstant(t time.Time) {
	t = t.UTC().Truncate(time.Second)
	if slices.Contains(r.Instants, t) {
		return
	}
	r.Instants = append(r.Instants, t)
	if over := len(r.Instants) - maxInstants; over > 0 {
		r.Instants = slices.Delete(r.Instants, 0, over)
	}
}
