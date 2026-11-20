package tempo_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func servicesRequest() datasource.ServiceRequest {
	to := time.Unix(1_800_000_000, 0)
	return datasource.ServiceRequest{Range: datasource.TimeRange{From: to.Add(-time.Hour), To: to}, Limit: 50}
}

func TestServicesReadsTheServiceTagOverTheWindow(t *testing.T) {
	ds, fake := serve(t, map[string]reply{servicesPath: {0, tagValuesBody("shop", "checkout", "checkout")}})
	req := servicesRequest()
	got, err := ds.Services(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := datasource.ServiceList{Signal: "traces", Field: "resource.service.name", Values: []string{"checkout", "shop"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Services = %+v, want %+v", got, want)
	}
	if len(fake.requests) != 1 || fake.requests[0].params.Get("start") == "" || fake.requests[0].params.Get("end") == "" {
		t.Fatalf("requests = %+v, want one bounded by start and end", fake.requests)
	}
}

func TestServicesRefusesLogs(t *testing.T) {
	ds, _ := serve(t, nil)
	req := servicesRequest()
	req.Signal = "logs"
	if _, err := ds.Services(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want ErrInvalidQuery", err)
	}
}

func TestServicesPassesAnUpstreamFailureOn(t *testing.T) {
	ds, _ := serve(t, map[string]reply{servicesPath: {500, `{}`}})
	if _, err := ds.Services(context.Background(), servicesRequest()); err == nil {
		t.Fatal("want an error")
	}
}
