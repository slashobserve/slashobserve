package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func servicesRequest(signal string) datasource.ServiceRequest {
	return datasource.ServiceRequest{Signal: signal, Range: datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}, Limit: 50}
}

func TestServicesAggregatesTheMappingsServiceField(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_search", 200, termsReply)
	ds := serve(t, f)
	got, err := ds.Services(context.Background(), servicesRequest(""))
	if err != nil {
		t.Fatal(err)
	}
	if got.Signal != "logs" || got.Field != "service.name" || !slices.Equal(got.Values, []string{"500", "checkout", "frontend"}) {
		t.Fatalf("Services = %+v", got)
	}
	if field := termsField(t, f.last(t)); field != "service.name" {
		t.Fatalf("terms field = %s, want service.name", field)
	}
	if n := f.count("/logs-*/_search"); n != 1 {
		t.Fatalf("%d searches, want 1", n)
	}
}

func TestServicesRefusesTraces(t *testing.T) {
	ds := serve(t, healthyServer(t))
	if _, err := ds.Services(context.Background(), servicesRequest("traces")); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
}
