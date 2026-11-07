package datasource

import "testing"

func TestSpanLinksJSON(t *testing.T) {
	if got, err := SpanLinksJSON(nil); err != nil || got != "[]" {
		t.Fatalf("no links = %q, %v; want []", got, err)
	}
	links := []SpanLink{
		{TraceID: "0af7651916cd43dd8448eb211c80319c", SpanID: "b7ad6b7169203331"},
		{TraceID: "00f067aa0ba902b700f067aa0ba902b7", SpanID: "00f067aa0ba902b7", Attributes: map[string]any{"kind": "follows_from"}},
	}
	got, err := SpanLinksJSON(links)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"trace_id":"0af7651916cd43dd8448eb211c80319c","span_id":"b7ad6b7169203331","attributes":{}},` +
		`{"trace_id":"00f067aa0ba902b700f067aa0ba902b7","span_id":"00f067aa0ba902b7","attributes":{"kind":"follows_from"}}]`
	if got != want {
		t.Fatalf("links = %s\nwant %s", got, want)
	}
}

func TestSpanStatusName(t *testing.T) {
	cases := map[string]string{"1": "Ok", "STATUS_CODE_OK": "Ok", "Ok": "Ok", "2": "Error", "STATUS_CODE_ERROR": "Error", "Error": "Error", "0": "Unset", "STATUS_CODE_UNSET": "Unset", "": "Unset"}
	for raw, want := range cases {
		if got := SpanStatusName(raw); got != want {
			t.Errorf("SpanStatusName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestCompareTraceIDsLowercasesDeduplicatesAndRefusesOverlap(t *testing.T) {
	upper := "0AF7651916CD43DD8448EB211C80319C"
	sel, base, err := CompareTraceIDs([]string{upper, "0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331"}, []string{"00f067aa0ba902b7"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sel) != 2 || sel[0] != "0af7651916cd43dd8448eb211c80319c" || sel[1] != "b7ad6b7169203331" || len(base) != 1 {
		t.Fatalf("sel = %v, base = %v", sel, base)
	}
	many := make([]string, MaxCompareSelected+1)
	for i := range many {
		many[i] = "0af7651916cd43dd8448eb211c80319c"
	}
	cases := []struct {
		name               string
		selected, baseline []string
		want               string
	}{
		{"empty selected", nil, []string{"b7ad6b7169203331"}, "selected must hold from 1 to 50 trace ids"},
		{"too many selected", many, []string{"b7ad6b7169203331"}, "selected must hold from 1 to 50 trace ids"},
		{"empty baseline", []string{"b7ad6b7169203331"}, []string{}, "baseline must hold from 1 to 150 trace ids"},
		{"bad id", []string{"abc"}, []string{"b7ad6b7169203331"}, "selected must hold trace ids of 16 or 32 hex characters"},
		{"overlap", []string{"b7ad6b7169203331"}, []string{"B7AD6B7169203331"}, "a trace id cannot be in both selected and baseline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := CompareTraceIDs(tc.selected, tc.baseline); err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
