package loki_test

import (
	"context"
	"testing"
)

func TestTraceLogsSendAServiceNameWithADollarVerbatim(t *testing.T) {
	ds, fake := serveTraceLogs(t, []string{"service_name"}, nil)

	if _, err := ds.TraceLogs(context.Background(), traceRequest("cost$center", "")); err != nil {
		t.Fatal(err)
	}

	want := `{service_name="cost$center"} | trace_id="` + traceID + `"`
	if got := fake.sent(); len(got) == 0 || got[0] != want {
		t.Fatalf("queries = %q, want first %q", got, want)
	}
}
