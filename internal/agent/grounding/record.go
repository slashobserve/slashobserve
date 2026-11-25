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
)

const (
	MaxValues       = 2000
	maxValuesOfRead = 500
	maxNames        = 400
	maxNameBytes    = 64
	maxRanges       = 64
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
	Mode   Mode      `json:"mode"`
	Values []float64 `json:"values,omitempty"`
	Names  []string  `json:"names,omitempty"`
	Ranges []Range   `json:"ranges,omitempty"`
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
	for _, v := range r.Values {
		read.seen[v] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var parsed any
	if decoder.Decode(&parsed) == nil {
		read.walk(parsed)
	}
	if over := len(r.Values) - MaxValues; over > 0 {
		r.Values = slices.Delete(r.Values, 0, over)
	}
	for _, name := range names {
		r.addName(name)
	}
	for _, rng := range ranges {
		r.addRange(rng)
	}
}

type reading struct {
	record *Record
	seen   map[float64]bool
	added  int
}

func (rd *reading) walk(v any) {
	switch v := v.(type) {
	case map[string]any:
		for key, child := range v {
			rd.text(key, false)
			rd.walk(child)
		}
	case []any:
		for _, child := range v {
			rd.walk(child)
		}
	case json.Number:
		if f, err := strconv.ParseFloat(string(v), 64); err == nil {
			rd.value(f)
		}
	case string:
		rd.text(v, true)
	}
}

func (rd *reading) text(s string, named bool) {
	if len(s) > maxScannedBytes {
		s = s[:maxScannedBytes]
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		rd.value(f)
		return
	}
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return
	}
	if named {
		if identifier.MatchString(s) {
			rd.record.addName(s)
		}
		for _, m := range quotedValue.FindAllStringSubmatch(s, -1) {
			rd.record.addName(m[1])
		}
	}
	for _, f := range valuesIn(s) {
		rd.value(f)
	}
}

func (rd *reading) value(v float64) {
	v = rounded(v)
	if math.IsNaN(v) || math.IsInf(v, 0) || rd.seen[v] || rd.added >= maxValuesOfRead {
		return
	}
	rd.seen[v] = true
	rd.added++
	rd.record.Values = append(rd.record.Values, v)
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
