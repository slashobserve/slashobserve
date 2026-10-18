package proxyhttp

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func refA(frames []datasource.Frame) []queryproxy.QueryResult {
	return []queryproxy.QueryResult{{Ref: "A", Frames: frames}}
}

func TestMarshalFramesConvertsTimesToEpochMilliseconds(t *testing.T) {
	ts := time.Date(2026, 10, 18, 10, 0, 0, 123_000_000, time.UTC)
	frames := []datasource.Frame{{
		Name:   "up",
		Labels: map[string]string{"pod": "a"},
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{ts, ts.Add(time.Second)}},
			{Name: "Value", Type: datasource.FieldNumber, Values: []any{1.5, 2.0}},
		},
	}}
	body, err := marshalFrames(refA(frames))
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"ref":"A","name":"up","labels":{"pod":"a"},"fields":[{"name":"Time","type":"time","values":[1792317600123,1792317601123]},{"name":"Value","type":"number","values":[1.5,2]}]}]`
	if string(body) != want {
		t.Fatalf("body = %s\nwant   %s", body, want)
	}
}

func TestMarshalFramesRendersNaNAndInfAsNull(t *testing.T) {
	frames := []datasource.Frame{{Fields: []datasource.Field{{
		Name: "Value", Type: datasource.FieldNumber,
		Values: []any{math.NaN(), math.Inf(1), math.Inf(-1), float32(math.NaN()), nil, 3.0},
	}}}}
	body, err := marshalFrames(refA(frames))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"values":[null,null,null,null,null,3]`; !strings.Contains(string(body), want) {
		t.Fatalf("body = %s, want it to contain %s", body, want)
	}
}

func TestMarshalFramesOmitsEmptyNameAndLabels(t *testing.T) {
	body, err := marshalFrames(refA([]datasource.Frame{{Fields: []datasource.Field{{Name: "n", Type: datasource.FieldString, Values: []any{"x"}}}}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"labels"`) || strings.Contains(string(body), `"name":""`) {
		t.Fatalf("empty name or labels were written: %s", body)
	}
}

func TestMarshalFramesEmptyResultIsAnEmptyArray(t *testing.T) {
	body, err := marshalFrames(nil)
	if err != nil || string(body) != "[]" {
		t.Fatalf("body = %s, err = %v", body, err)
	}
}

func TestMarshalFramesKeepsOtherValuesAsProduced(t *testing.T) {
	frames := []datasource.Frame{{Fields: []datasource.Field{{Name: "o", Type: datasource.FieldOther, Values: []any{map[string]any{"a": 1}, "s", true}}}}}
	body, err := marshalFrames(refA(frames))
	if err != nil {
		t.Fatal(err)
	}
	var decoded []wireFrame
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := len(decoded[0].Fields[0].Values); got != 3 {
		t.Fatalf("%d values, want 3", got)
	}
}

func TestMarshalFramesRejectsWronglyTypedTimeField(t *testing.T) {
	for name, values := range map[string][]any{
		"string": {"2026-10-18T10:00:00Z"},
		"number": {1.0},
		"int":    {int64(1)},
	} {
		frames := []datasource.Frame{{Fields: []datasource.Field{{Name: "Time", Type: datasource.FieldTime, Values: values}}}}
		if _, err := marshalFrames(refA(frames)); err == nil {
			t.Errorf("%s value in a time field was accepted", name)
		}
	}
}

func TestMarshalFramesRejectsTimeValueInNonTimeField(t *testing.T) {
	frames := []datasource.Frame{{Fields: []datasource.Field{{Name: "Value", Type: datasource.FieldNumber, Values: []any{time.Now()}}}}}
	if _, err := marshalFrames(refA(frames)); err == nil {
		t.Fatal("a time value in a number field was accepted")
	}
}

func TestMarshalFramesTagsEachFrameWithItsQueryRefInPanelOrder(t *testing.T) {
	frame := func(name string) datasource.Frame {
		return datasource.Frame{Name: name, Fields: []datasource.Field{{Name: "v", Type: datasource.FieldNumber, Values: []any{1.0}}}}
	}
	body, err := marshalFrames([]queryproxy.QueryResult{
		{Ref: "A", Frames: []datasource.Frame{frame("a1"), frame("a2")}},
		{Ref: "errors", Frames: nil},
		{Ref: "D", Frames: []datasource.Frame{frame("d1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []wireFrame
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range decoded {
		got = append(got, f.Ref+":"+f.Name)
	}
	if want := "A:a1 A:a2 D:d1"; strings.Join(got, " ") != want {
		t.Fatalf("frames = %v, want %s", got, want)
	}
}
