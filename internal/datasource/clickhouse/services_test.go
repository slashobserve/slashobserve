package clickhouse

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func servicesReq(signal string) datasource.ServiceRequest {
	end := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	return datasource.ServiceRequest{Signal: signal, Range: datasource.TimeRange{From: end.Add(-time.Hour), To: end}, Limit: 50}
}

func TestServicesReadsDistinctServiceNamesOfTheSignalsTable(t *testing.T) {
	for signal, table := range map[string]string{"": "otel_logs", "logs": "otel_logs", "traces": "otel_traces"} {
		conn := &fakeConn{rows: [][]any{{"checkout"}, {"shop"}}}
		got, err := facetsDS(conn).Services(context.Background(), servicesReq(signal))
		if err != nil {
			t.Fatalf("%q: %v", signal, err)
		}
		wantSignal := signal
		if wantSignal == "" {
			wantSignal = "logs"
		}
		want := datasource.ServiceList{Signal: wantSignal, Field: "ServiceName", Values: []string{"checkout", "shop"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: Services = %+v, want %+v", signal, got, want)
		}
		sent := conn.sent()
		if len(sent) != 1 || !strings.Contains(sent[0], " FROM "+table+" WHERE Timestamp >= ") || !strings.Contains(sent[0], "ServiceName != ''") {
			t.Fatalf("%q: statements = %v, want one over %s", signal, sent, table)
		}
	}
}

func TestServicesRefusesAnUnknownSignal(t *testing.T) {
	_, err := facetsDS(&fakeConn{}).Services(context.Background(), servicesReq("metrics"))
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
}
