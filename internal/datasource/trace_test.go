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
