package proxyhttp

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestMarshalExemplarsTagsFramesWithTheQueryRefAndConvertsTimes(t *testing.T) {
	q := queryproxy.QueryResult{Ref: "errors", Exemplars: []datasource.Frame{{
		Name:   "m",
		Labels: map[string]string{"pod": "a"},
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{time.UnixMilli(1760000000123).UTC()}},
			{Name: "Value", Type: datasource.FieldNumber, Values: []any{math.NaN()}},
			{Name: "trace_id", Type: datasource.FieldString, Values: []any{"abc"}},
		},
	}}}
	body, err := marshalExemplars(q)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"ref":"errors","name":"m","labels":{"pod":"a"},"fields":[{"name":"Time","type":"time","values":[1760000000123]},{"name":"Value","type":"number","values":[null]},{"name":"trace_id","type":"string","values":["abc"]}]}]`
	if string(body) != want {
		t.Fatalf("exemplars = %s, want %s", body, want)
	}
	if empty, err := marshalExemplars(queryproxy.QueryResult{Ref: "A"}); err != nil || string(empty) != "[]" {
		t.Fatalf("no exemplars = %s, %v, want []", empty, err)
	}
}

func TestJoinFramesAllocatesExactlyWhatItHolds(t *testing.T) {
	parts := [][]byte{
		[]byte(`[{"a":1},{"a":2}]`),
		[]byte(`[]`),
		[]byte(strings.Repeat(`[{"b":"`+strings.Repeat("x", 1000)+`"}]`, 1)),
		[]byte(`[{"c":3}]`),
	}
	got := joinFrames(parts)
	want := `[{"a":1},{"a":2},{"b":"` + strings.Repeat("x", 1000) + `"},{"c":3}]`
	if string(got) != want {
		t.Fatalf("joined frames differ: %s", got)
	}
	if cap(got) != len(got) {
		t.Fatalf("cap %d for len %d: the cache charges len, so spare capacity is memory it does not count", cap(got), len(got))
	}
	if empty := joinFrames(nil); string(empty) != "[]" || cap(empty) != len(empty) {
		t.Fatalf("empty join = %q cap %d", empty, cap(empty))
	}
}
