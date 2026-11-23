package queryproxy

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type servicesFake struct {
	*fakeDatasource
	calls  atomic.Int32
	reply  func(call int32) ([]string, error)
	ranges chan datasource.TimeRange
	hang   bool
}

func (f *servicesFake) Services(ctx context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	n := f.calls.Add(1)
	if f.hang {
		select {
		case <-ctx.Done():
			return datasource.ServiceList{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	if f.ranges != nil {
		f.ranges <- req.Range
	}
	values, err := f.reply(n)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	return datasource.ServiceList{Signal: "logs", Field: "service_name", Values: values}, nil
}

func servicesRunner(t *testing.T, fake *servicesFake, opts ...RunnerOption) *Runner {
	t.Helper()
	runner := newRunnerOf(t, discardLogger(), datasource.Instance{UID: "loki", Datasource: fake})
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

func newServicesFake(reply func(call int32) ([]string, error)) *servicesFake {
	return &servicesFake{fakeDatasource: newFake("loki"), reply: reply}
}

func fixedServices(names ...string) func(int32) ([]string, error) {
	return func(int32) ([]string, error) { return names, nil }
}

func TestServicesAnswersFromTheCacheForTheSameSourceSignalAndWindow(t *testing.T) {
	fake := newServicesFake(fixedServices("checkout", "shop"))
	runner := servicesRunner(t, fake)
	req := ServicesRequest{Datasource: "loki", Signal: "logs", Window: time.Hour}

	first, err := runner.Services(reading(context.Background()), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Services(reading(context.Background()), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("source asked %d times, want 1", got)
	}
	if !slices.Equal(first.Services, second.Services) || first.Field != "service_name" || first.Signal != "logs" {
		t.Fatalf("answers differ: %+v and %+v", first, second)
	}

	other := req
	other.Window = 15 * time.Minute
	if _, err := runner.Services(reading(context.Background()), other); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("source asked %d times after a new window, want 2", got)
	}
}

func TestServicesDefaultsTheWindowToOneHourEndingNow(t *testing.T) {
	fake := newServicesFake(fixedServices("shop"))
	fake.ranges = make(chan datasource.TimeRange, 1)
	runner := servicesRunner(t, fake)
	before := time.Now()
	got, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki"})
	if err != nil {
		t.Fatal(err)
	}
	rng := <-fake.ranges
	if got.Window != time.Hour || rng.To.Before(before) || rng.To.Sub(rng.From) != time.Hour {
		t.Fatalf("window = %v, range = %v to %v", got.Window, rng.From, rng.To)
	}
}

func TestServicesOfAPinnedClockEndAtItAndKeepTheirOwnCacheEntry(t *testing.T) {
	fake := newServicesFake(fixedServices("shop"))
	fake.ranges = make(chan datasource.TimeRange, 3)
	runner := servicesRunner(t, fake)
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	pinned := datasource.WithNow(reading(context.Background()), at)
	req := ServicesRequest{Datasource: "loki"}

	if _, err := runner.Services(pinned, req); err != nil {
		t.Fatal(err)
	}
	if rng := <-fake.ranges; !rng.To.Equal(at) || rng.To.Sub(rng.From) != time.Hour {
		t.Fatalf("range = %v to %v, want the hour ending %s", rng.From, rng.To, at)
	}
	if _, err := runner.Services(pinned, req); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("source asked %d times for the same pinned clock, want 1", got)
	}
	if _, err := runner.Services(reading(context.Background()), req); err != nil {
		t.Fatal(err)
	}
	if rng := <-fake.ranges; rng.To.Equal(at) {
		t.Fatalf("a live read ended at the pinned clock %s", rng.To)
	}
	other := datasource.WithNow(reading(context.Background()), at.Add(time.Hour))
	if _, err := runner.Services(other, req); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("source asked %d times across three clocks, want 3", got)
	}
}

func TestServicesCutsAtTheLimitAndSaysSo(t *testing.T) {
	names := make([]string, MaxServices+1)
	for i := range names {
		names[i] = "svc-" + strconv.Itoa(i)
	}
	got, err := servicesRunner(t, newServicesFake(fixedServices(names...))).Services(reading(context.Background()), ServicesRequest{Datasource: "loki"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != MaxServices || !got.Truncated {
		t.Fatalf("got %d services, truncated = %v", len(got.Services), got.Truncated)
	}
}

func TestServicesRefusesAWindowOutsideTheBounds(t *testing.T) {
	runner := servicesRunner(t, newServicesFake(fixedServices()))
	for _, window := range []time.Duration{time.Second, MaxServicesWindow + time.Second, -time.Hour} {
		_, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki", Window: window})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("window %v: err = %v, want ErrInvalidRequest", window, err)
		}
	}
}

func TestServicesRefusesADatasourceThatHoldsNone(t *testing.T) {
	runner := newRunnerOf(t, discardLogger(), datasource.Instance{UID: "prom", Datasource: newFake("prometheus")})
	_, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "prom"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
	if _, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "nope"}); !errors.Is(err, ErrUnknownDatasource) {
		t.Fatalf("err = %v, want ErrUnknownDatasource", err)
	}
}

func TestServicesOfASlowSourceTimesOutInsteadOfBlocking(t *testing.T) {
	fake := newServicesFake(fixedServices("shop"))
	fake.hang = true
	runner := servicesRunner(t, fake, WithServicesTimeout(30*time.Millisecond))
	start := time.Now()
	_, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki"})
	if status, code, _ := Classify(err); status != http.StatusGatewayTimeout || code != "query_timeout" {
		t.Fatalf("Classify(%v) = %d %s, want 504 query_timeout", err, status, code)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("waited %v", elapsed)
	}
}

func TestServicesDoesNotCacheAFailure(t *testing.T) {
	fake := newServicesFake(func(call int32) ([]string, error) {
		if call == 1 {
			return nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
		}
		return []string{"shop"}, nil
	})
	runner := servicesRunner(t, fake)
	if _, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki"}); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	got, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki"})
	if err != nil || !slices.Equal(got.Services, []string{"shop"}) {
		t.Fatalf("retry = %+v, %v", got, err)
	}
}

func TestServicesReportsABusyUpstreamAfterTheWaitBound(t *testing.T) {
	fake := newServicesFake(fixedServices("shop"))
	runner := servicesRunner(t, fake, WithUpstreamWait(30*time.Millisecond))
	runner.slots = NewSlots(map[string]int{"loki": 1}, nil)
	release, err := runner.AcquireUpstream(reading(context.Background()), "loki")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := runner.Services(reading(context.Background()), ServicesRequest{Datasource: "loki"}); !errors.Is(err, ErrUpstreamBusy) {
		t.Fatalf("err = %v, want ErrUpstreamBusy", err)
	}
	if got := fake.calls.Load(); got != 0 {
		t.Fatalf("source asked %d times without a slot", got)
	}
}
