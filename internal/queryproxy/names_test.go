package queryproxy

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func namesRunner(t *testing.T, fake *fakeDatasource, opts ...RunnerOption) *Runner {
	t.Helper()
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ds": fake})
	runner.sem = semaphore.NewWeighted(1)
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

func TestNamesReportsABusyUpstreamAfterTheWaitBound(t *testing.T) {
	fake := newFake("prometheus")
	runner := namesRunner(t, fake, WithUpstreamWait(30*time.Millisecond))
	release, err := runner.AcquireUpstream(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics})
	if !errors.Is(err, ErrUpstreamBusy) {
		t.Fatalf("err = %v, want ErrUpstreamBusy", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v for a slot", elapsed)
	}
	status, code, message := Classify(err)
	if status != http.StatusTooManyRequests || code != "too_many_requests" || message == "" {
		t.Fatalf("Classify = (%d, %q, %q)", status, code, message)
	}
	if got := len(fake.discoverRequests()); got != 0 {
		t.Fatalf("discovery ran %d times without a slot", got)
	}

	release()
	if _, err := runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics}); err != nil {
		t.Fatalf("Names after release: %v", err)
	}
}

func TestNamesKeepsTheCallersCancellationApartFromABusyUpstream(t *testing.T) {
	runner := namesRunner(t, newFake("prometheus"), WithUpstreamWait(time.Minute))
	release, err := runner.AcquireUpstream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err = runner.Names(ctx, NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics})
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUpstreamBusy) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
