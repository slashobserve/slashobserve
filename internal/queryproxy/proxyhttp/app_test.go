package proxyhttp

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestPanelWindowMatchesTheDashboardRoute(t *testing.T) {
	d := &spec.Dashboard{Time: spec.TimeRange{From: "now-6h", To: "now"}, Timezone: "UTC"}
	panel := &spec.Panel{ID: "p", Type: spec.Timeseries, TimeShift: "1h", MaxDataPoints: 100}
	now := at(777 * time.Millisecond)

	window, err := ResolvePanelWindow(d, panel, "", "", 500, now)
	if err != nil {
		t.Fatal(err)
	}
	route, err := params{maxDataPoints: 500}.panelWindow(d, panel, now)
	if err != nil {
		t.Fatal(err)
	}
	if window.Range != route.rng || window.MaxDataPoints != route.maxDataPoints {
		t.Fatalf("window %+v differs from the route's %+v", window, route)
	}
	if window.Range.To.Nanosecond() != 0 || window.Range.From.Nanosecond() != 0 {
		t.Errorf("range %v is not aligned to whole seconds", window.Range)
	}
	if !window.Range.To.Before(now.Add(-time.Hour).Add(time.Second)) {
		t.Errorf("to %v does not carry the one hour shift of %v", window.Range.To, now)
	}
}

func TestPanelWindowRefusesMoreThanFourHundredDays(t *testing.T) {
	d := &spec.Dashboard{Time: spec.TimeRange{From: "now-1h", To: "now"}, Timezone: "UTC"}
	_, err := ResolvePanelWindow(d, &spec.Panel{ID: "p", Type: spec.Timeseries}, "now-401d", "now", 200, at(0))
	if !errors.Is(err, queryproxy.ErrInvalidRequest) {
		t.Fatalf("err = %v, want an invalid request", err)
	}
}
