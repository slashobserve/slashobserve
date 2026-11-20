package loki_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

func servicesRequest() datasource.ServiceRequest {
	to := time.Unix(1_800_000_000, 0)
	return datasource.ServiceRequest{Range: datasource.TimeRange{From: to.Add(-time.Hour), To: to}, Limit: 50}
}

func TestServicesReadsServiceNameInOneRequestOverTheWindow(t *testing.T) {
	ds, fake := serveRoutes(t, map[string]string{"/loki/api/v1/label/service_name/values": labelList("shop", "checkout", "checkout")})
	got, err := ds.Services(context.Background(), servicesRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := datasource.ServiceList{Signal: "logs", Field: "service_name", Values: []string{"checkout", "shop"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Services = %+v, want %+v", got, want)
	}
	requests := fake.to("/loki/api/v1/label/service_name/values")
	if len(requests) != 1 || requests[0].params.Get("start") == "" || requests[0].params.Get("end") == "" {
		t.Fatalf("requests = %+v, want one bounded by start and end", requests)
	}
	if n := len(fake.requests); n != 1 {
		t.Fatalf("%d requests, want 1", n)
	}
}

func TestServicesFallsBackToTheFirstOtherServiceLabelThatHasValues(t *testing.T) {
	ds, _ := serveRoutes(t, map[string]string{
		"/loki/api/v1/label/service_name/values": labelList(),
		"/loki/api/v1/labels":                    labelList("app", "job", "level"),
		"/loki/api/v1/label/app/values":          labelList("payments"),
		"/loki/api/v1/label/job/values":          labelList("batch"),
	})
	got, err := ds.Services(context.Background(), servicesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Field != "app" || !reflect.DeepEqual(got.Values, []string{"payments"}) {
		t.Fatalf("Services = %+v, want field app with payments", got)
	}
}

func TestServicesWithNoServiceLabelIsEmptyAndNamesTheDefaultField(t *testing.T) {
	ds, _ := serveRoutes(t, map[string]string{
		"/loki/api/v1/label/service_name/values": labelList(),
		"/loki/api/v1/labels":                    labelList("level"),
	})
	got, err := ds.Services(context.Background(), servicesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Field != "service_name" || len(got.Values) != 0 {
		t.Fatalf("Services = %+v, want no values under service_name", got)
	}
}

func TestServicesRefusesTraces(t *testing.T) {
	ds, _ := serveRoutes(t, nil)
	req := servicesRequest()
	req.Signal = "traces"
	if _, err := ds.Services(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
}

func TestServiceLabelsStartWithServiceName(t *testing.T) {
	if loki.ServiceLabels[0] != "service_name" {
		t.Fatalf("ServiceLabels = %v", loki.ServiceLabels)
	}
}
