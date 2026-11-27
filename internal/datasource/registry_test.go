package datasource_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func everything(t *testing.T) context.Context {
	return datasource.WithAccess(t.Context(), datasource.Access{System: true})
}

func lookup(t *testing.T, registry *datasource.Registry, uid string) (datasource.Instance, bool) {
	in, err := registry.For(everything(t), uid)
	return in, err == nil
}

type failingHealth struct {
	datasource.Datasource
	err error
}

func (f failingHealth) Type() string                      { return "prometheus" }
func (f failingHealth) CheckHealth(context.Context) error { return f.err }
func (f failingHealth) Close() error                      { return nil }

func TestHealthMessageNeverCarriesTheDatasourceAddress(t *testing.T) {
	address := "http://10.1.2.3:9090"
	transport := &url.Error{Op: "Post", URL: address + "/api/v1/query", Err: fmt.Errorf("dial tcp 10.1.2.3:9090: %w", syscall.ECONNREFUSED)}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"transport failure", transport, "The health check failed"},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "upstream %s unreachable", address), "The datasource is not answering"},
		{"timeout", datasource.Errorf(datasource.ErrTimeout, "%s timed out", address), "Timed out waiting for the datasource"},
		{"forbidden", datasource.Errorf(datasource.ErrForbiddenQuery, "user bob at %s", address), "The datasource refused the health check"},
		{"not read-only", datasource.Errorf(datasource.ErrNotReadOnly, "user bob at %s", address), "The ClickHouse user must be readonly = 2"},
		{"unsafe grant", datasource.Errorf(datasource.ErrUnsafeGrant, "user bob at %s holds FILE", address), "The ClickHouse user holds a grant that reaches outside systems; revoke it"},
		{"auth failed", datasource.Errorf(datasource.ErrAuthFailed, "user bob at %s refused", address), "The datasource refused the credentials"},
		{"host not allowed", datasource.Errorf(datasource.ErrHostNotAllowed, "user bob at %s", address), "The ClickHouse user may not connect from this server's address"},
		{"limit", datasource.Errorf(datasource.ErrLimitExceeded, "%s sent too much", address), "The health check hit a datasource limit"},
		{"invalid", datasource.Errorf(datasource.ErrInvalidQuery, "%s rejected", address), "The datasource rejected the health check"},
		{"is opensearch", datasource.Errorf(datasource.ErrIsOpenSearch, "%s is opensearch", address), "The server is OpenSearch, configure it as type opensearch"},
		{"is elasticsearch", datasource.Errorf(datasource.ErrIsElasticsearch, "%s is elasticsearch", address), "The server is Elasticsearch, configure it as type elasticsearch"},
		{"unsupported version", datasource.Errorf(datasource.ErrUnsupportedVersion, "%s runs 7.10", address), "The datasource version is not supported: Elasticsearch 8.x or 9.x, OpenSearch 2.x or 3.x"},
		{"no index", datasource.Errorf(datasource.ErrNoIndex, "%s has no logs-*", address), "The index pattern matches no index"},
		{"no time field", datasource.Errorf(datasource.ErrNoTimeField, "%s lacks @timestamp", address), "The time field is missing or is not a date"},
		{"no role field", datasource.Errorf(datasource.ErrNoRoleField, "%s lacks severity", address), "A configured field is missing from the mapping"},
		{"deadline", fmt.Errorf("%s: %w", address, context.DeadlineExceeded), "Timed out waiting for the datasource"},
		{"wrapped", errors.Join(errors.New(address), datasource.Errorf(nil, "%s down", address)), "The datasource is not answering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: failingHealth{err: tt.err}})
			if err != nil {
				t.Fatal(err)
			}
			statuses := registry.Health(everything(t))
			if len(statuses) != 1 || statuses[0].Healthy {
				t.Fatalf("statuses = %+v", statuses)
			}
			message := statuses[0].Message
			if message != tt.want {
				t.Errorf("message = %q, want %q", message, tt.want)
			}
			for _, leak := range []string{"10.1.2.3", "9090", "http://", "bob"} {
				if strings.Contains(message, leak) {
					t.Errorf("message %q contains %q", message, leak)
				}
			}
		})
	}
}

type blockedHealth struct {
	datasource.Datasource
	release chan struct{}
}

func (b blockedHealth) Type() string { return "prometheus" }
func (b blockedHealth) CheckHealth(context.Context) error {
	<-b.release
	return nil
}
func (b blockedHealth) Close() error { return nil }

func TestHealthReportsACancelledRequestWithoutWaitingForTheCheck(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: blockedHealth{release: release}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(everything(t))
	cancel()
	statuses := registry.Health(ctx)
	if len(statuses) != 1 || statuses[0].Healthy || statuses[0].Message != "The health check was cancelled" {
		t.Fatalf("statuses = %+v", statuses)
	}
}

func TestRefinedKindsKeepTheirParentKind(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		parent error
	}{
		{"not read-only", datasource.Errorf(datasource.ErrNotReadOnly, "x"), datasource.ErrForbiddenQuery},
		{"unsafe grant", datasource.Errorf(datasource.ErrUnsafeGrant, "x"), datasource.ErrForbiddenQuery},
		{"auth failed", datasource.Errorf(datasource.ErrAuthFailed, "x"), datasource.ErrUnavailable},
		{"host not allowed", datasource.Errorf(datasource.ErrHostNotAllowed, "x"), datasource.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.parent) {
				t.Errorf("%v should match %v", tt.err, tt.parent)
			}
			if got, want := tt.err.Error(), tt.parent.Error()+": x"; got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
		})
	}
	if errors.Is(datasource.Errorf(datasource.ErrNotReadOnly, "x"), datasource.ErrUnsafeGrant) {
		t.Error("refined kinds should stay distinct from each other")
	}
}

type indexedHealth struct {
	failingHealth
}

func (indexedHealth) Type() string                       { return "elasticsearch" }
func (indexedHealth) Index() (pattern, timeField string) { return "logs-*", "@timestamp" }

func TestStatusCarriesTheIndexOfASearchDatasource(t *testing.T) {
	registry, err := datasource.NewRegistry(nil,
		datasource.Instance{UID: "es", Datasource: indexedHealth{}},
		datasource.Instance{UID: "es-down", Datasource: indexedHealth{failingHealth{err: datasource.Errorf(datasource.ErrNoIndex, "none")}}},
		datasource.Instance{UID: "prom", Datasource: failingHealth{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	keys := func(s datasource.Status) map[string]any {
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, s := range registry.Health(everything(t)) {
		m := keys(s)
		switch s.UID {
		case "es", "es-down":
			if m["index"] != "logs-*" || m["time_field"] != "@timestamp" || s.Healthy != (s.UID == "es") {
				t.Errorf("%s status = %+v", s.UID, s)
			}
		default:
			if _, ok := m["index"]; ok {
				t.Errorf("%s status carries an index: %v", s.UID, m)
			}
			if _, ok := m["time_field"]; ok {
				t.Errorf("%s status carries a time field: %v", s.UID, m)
			}
		}
	}

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	blocked, err := datasource.NewRegistry(nil, datasource.Instance{UID: "es", Datasource: indexedBlocked{blockedHealth{release: release}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(everything(t))
	cancel()
	statuses := blocked.Health(ctx)
	if len(statuses) != 1 || statuses[0].Healthy || statuses[0].Index != "logs-*" || statuses[0].TimeField != "@timestamp" {
		t.Fatalf("cancelled statuses = %+v", statuses)
	}
}

type indexedBlocked struct {
	blockedHealth
}

func (indexedBlocked) Type() string                       { return "elasticsearch" }
func (indexedBlocked) Index() (pattern, timeField string) { return "logs-*", "@timestamp" }

type flippingHealth struct {
	datasource.Datasource
	err   atomic.Pointer[error]
	calls atomic.Int32
}

func (f *flippingHealth) Type() string { return "loki" }
func (f *flippingHealth) CheckHealth(context.Context) error {
	f.calls.Add(1)
	if err := f.err.Load(); err != nil {
		return *err
	}
	return nil
}
func (f *flippingHealth) Close() error { return nil }

func TestHealthDetailNamesTheCauseWithoutTheAddress(t *testing.T) {
	address := "http://10.1.2.3:3100"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"connection failed", datasource.Errorf(datasource.ErrUnavailable, "connection failed"), "The server could not connect to the datasource's address"},
		{"http status", datasource.ErrorfStatus(datasource.ErrUnavailable, 503, "Ingester not ready at %s", address), "The datasource answered HTTP 503"},
		{"no detail", datasource.Errorf(datasource.ErrTimeout, "%s timed out", address), ""},
		{"not an upstream error", errors.New(address), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: failingHealth{err: tt.err}})
			if err != nil {
				t.Fatal(err)
			}
			got := registry.Health(everything(t))[0]
			if got.Detail != tt.want {
				t.Errorf("detail = %q, want %q", got.Detail, tt.want)
			}
			if strings.Contains(got.Detail, "10.1.2.3") {
				t.Errorf("detail %q carries the address", got.Detail)
			}
		})
	}
}

type fakeClock struct{ ns atomic.Int64 }

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.ns.Store(time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}

func (c *fakeClock) now() time.Time          { return time.Unix(0, c.ns.Load()).UTC() }
func (c *fakeClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

func registryWithClock(t *testing.T, ds datasource.Datasource) (*datasource.Registry, *fakeClock) {
	t.Helper()
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeClock()
	registry.SetClock(clock.now)
	return registry, clock
}

func TestRecheckRunsAFreshCheckAndThrottlesRepeats(t *testing.T) {
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, clock := registryWithClock(t, ds)
	if first := registry.Health(everything(t))[0]; first.Healthy {
		t.Fatalf("first status = %+v", first)
	}

	var recovered *error
	ds.err.Store(recovered)
	if cached := registry.Health(everything(t))[0]; cached.Healthy {
		t.Fatalf("a plain health read inside the cache window must not recheck: %+v", cached)
	}
	clock.advance(2100 * time.Millisecond)
	got, ok := registry.Recheck(everything(t), "loki")
	if !ok || !got.Healthy || got.Detail != "" {
		t.Fatalf("recheck = %+v, ok %v", got, ok)
	}
	calls := ds.calls.Load()
	if again, _ := registry.Recheck(everything(t), "loki"); !again.Healthy || ds.calls.Load() != calls {
		t.Errorf("a recheck inside the throttle window ran another check: calls %d -> %d", calls, ds.calls.Load())
	}
	if _, ok := registry.Recheck(everything(t), "missing"); ok {
		t.Error("recheck of an unknown uid reported found")
	}
}

type closeCounting struct {
	failingHealth
	closed atomic.Int32
}

func (c *closeCounting) Close() error {
	c.closed.Add(1)
	return nil
}

func TestPutAddsAndReplacesAppDatasources(t *testing.T) {
	fromFile := datasource.Instance{UID: "file", Datasource: failingHealth{}}
	registry, err := datasource.NewRegistry(nil, fromFile)
	if err != nil {
		t.Fatal(err)
	}
	first, second := &closeCounting{}, &closeCounting{}

	if err := registry.Put(datasource.Instance{UID: "added", Name: "One", Datasource: first}); err != nil {
		t.Fatal(err)
	}
	in, ok := lookup(t, registry, "added")
	if !ok || in.Source != datasource.SourceApp || in.Name != "One" {
		t.Fatalf("instance = %+v, ok = %v", in, ok)
	}
	if err := registry.Put(datasource.Instance{UID: "added", Name: "Two", Datasource: second}); err != nil {
		t.Fatal(err)
	}
	if in, _ := lookup(t, registry, "added"); in.Name != "Two" {
		t.Fatalf("name after replace = %q", in.Name)
	}
	if first.closed.Load() != 0 {
		t.Fatal("the replaced adapter closed before the grace period")
	}
	_ = registry.Close()
	if first.closed.Load() != 1 || second.closed.Load() != 1 {
		t.Fatalf("closed first=%d second=%d, want each once", first.closed.Load(), second.closed.Load())
	}
}

func TestPutAndRemoveLeaveFileDatasourcesAlone(t *testing.T) {
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "file", Name: "From file", Datasource: failingHealth{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(datasource.Instance{UID: "file", Datasource: failingHealth{}}); !errors.Is(err, datasource.ErrServerOwned) {
		t.Fatalf("Put over a file uid = %v, want ErrServerOwned", err)
	}
	if registry.Remove("file") {
		t.Fatal("Remove took a file datasource")
	}
	if in, ok := lookup(t, registry, "file"); !ok || in.Source != datasource.SourceFile || in.Name != "From file" {
		t.Fatalf("instance = %+v, ok = %v", in, ok)
	}
}

func TestPutForgetsTheCachedHealthOfTheReplacedDatasource(t *testing.T) {
	registry, err := datasource.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	broken := datasource.Instance{UID: "added", Datasource: failingHealth{err: datasource.Errorf(datasource.ErrUnavailable, "down")}}
	if err := registry.Put(broken); err != nil {
		t.Fatal(err)
	}
	if got := registry.Health(everything(t)); got[0].Healthy {
		t.Fatalf("status = %+v", got[0])
	}
	if err := registry.Put(datasource.Instance{UID: "added", Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	if got := registry.Health(everything(t)); !got[0].Healthy {
		t.Fatalf("an edited datasource still reports the cached failure: %+v", got[0])
	}
}

func TestRemoveDropsAnAppDatasource(t *testing.T) {
	registry, err := datasource.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(datasource.Instance{UID: "added", Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	if !registry.Remove("added") {
		t.Fatal("Remove found nothing")
	}
	if _, ok := lookup(t, registry, "added"); ok || len(registry.Health(everything(t))) != 0 {
		t.Fatal("a removed datasource is still listed")
	}
	if registry.Remove("added") {
		t.Fatal("Remove reported a second removal")
	}
}

type signalledHealth struct {
	datasource.Datasource
	started chan struct{}
	release chan struct{}
	err     error
}

func (s signalledHealth) Type() string { return "prometheus" }
func (s signalledHealth) CheckHealth(context.Context) error {
	close(s.started)
	<-s.release
	return s.err
}
func (s signalledHealth) Close() error { return nil }

func TestACheckOfAReplacedDatasourceNeitherCachesNorBlocksTheNewOne(t *testing.T) {
	registry, err := datasource.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := signalledHealth{started: make(chan struct{}), release: make(chan struct{}), err: datasource.Errorf(datasource.ErrUnavailable, "down")}
	if err := registry.Put(datasource.Instance{UID: "added", Datasource: old}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		registry.Health(everything(t))
	}()
	<-old.started

	if err := registry.Put(datasource.Instance{UID: "added", Datasource: failingHealth{}}); err != nil {
		t.Fatal(err)
	}
	rechecked := make(chan datasource.Status, 1)
	go func() {
		status, _ := registry.Recheck(everything(t), "added")
		rechecked <- status
	}()
	select {
	case status := <-rechecked:
		if !status.Healthy {
			t.Fatalf("a recheck after the edit reported the old definition: %+v", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a recheck after the edit waited for the check of the old definition")
	}
	close(old.release)
	<-done
	if got := registry.Health(everything(t)); !got[0].Healthy {
		t.Fatalf("the finished check of the old definition replaced the cached status: %+v", got[0])
	}
}

func TestHealthHasNoRowForADatasourceTheCallerCannotQuery(t *testing.T) {
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "open", Datasource: failingHealth{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: failingHealth{err: errors.New("down")}}); err != nil {
		t.Fatal(err)
	}
	uids := func(ctx context.Context) []string {
		var out []string
		for _, s := range registry.Health(ctx) {
			out = append(out, s.UID)
		}
		return out
	}
	editor := datasource.WithAccess(t.Context(), datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "e", Role: auth.RoleEditor}})
	admin := datasource.WithAccess(t.Context(), datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}})
	if got := uids(editor); len(got) != 1 || got[0] != "open" {
		t.Fatalf("editor health rows = %v, want only open", got)
	}
	if _, ok := registry.Recheck(editor, "audit"); ok {
		t.Fatal("an editor rechecked the admins only datasource")
	}
	if got := uids(admin); len(got) != 2 {
		t.Fatalf("admin health rows = %v, want both", got)
	}
}

func TestAnAnsweredQueryClearsACachedFailure(t *testing.T) {
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	if first := registry.Health(everything(t))[0]; first.Healthy {
		t.Fatalf("first status = %+v", first)
	}
	calls := ds.calls.Load()

	registry.Answered("loki")

	got := registry.Health(everything(t))[0]
	if !got.Healthy || got.Message != "" || got.Detail != "" {
		t.Fatalf("status after an answered query = %+v", got)
	}
	if ds.calls.Load() != calls {
		t.Errorf("clearing the failure ran another check: calls %d -> %d", calls, ds.calls.Load())
	}
}

func TestAnAnsweredQueryLeavesUnknownAndHealthyDatasourcesAlone(t *testing.T) {
	ds := &flippingHealth{}
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	registry.Answered("loki")
	registry.Answered("missing")
	if ds.calls.Load() != 0 {
		t.Fatalf("Answered ran a check: %d", ds.calls.Load())
	}
	if got := registry.Health(everything(t))[0]; !got.Healthy || ds.calls.Load() != 1 {
		t.Fatalf("status = %+v, checks = %d", got, ds.calls.Load())
	}
}

func TestAnAnsweredQueryKeepsAConfigurationFailure(t *testing.T) {
	for _, kind := range []error{datasource.ErrIsOpenSearch, datasource.ErrIsElasticsearch, datasource.ErrNoTimeField, datasource.ErrUnsupportedVersion, datasource.ErrNoIndex} {
		t.Run(kind.Error(), func(t *testing.T) {
			ds := &flippingHealth{}
			failure := datasource.Errorf(kind, "wrong setup")
			ds.err.Store(&failure)
			registry, _ := registryWithClock(t, ds)
			before := registry.Health(everything(t))[0]
			registry.Answered("loki")
			got := registry.Health(everything(t))[0]
			if got.Healthy || got.Message != before.Message || got.Message == "" {
				t.Fatalf("status after an answered query = %+v, want the failure %+v", got, before)
			}
		})
	}
}

func TestAnAnsweredQueryClearsEveryTransientFailure(t *testing.T) {
	for _, kind := range []error{datasource.ErrUnavailable, datasource.ErrTimeout, datasource.ErrLimitExceeded} {
		t.Run(kind.Error(), func(t *testing.T) {
			ds := &flippingHealth{}
			failure := datasource.Errorf(kind, "busy")
			ds.err.Store(&failure)
			registry, _ := registryWithClock(t, ds)
			registry.Health(everything(t))
			registry.Answered("loki")
			if got := registry.Health(everything(t))[0]; !got.Healthy {
				t.Fatalf("status = %+v", got)
			}
		})
	}
}

type gatedHealth struct {
	flippingHealth
	gate    chan struct{}
	entered atomic.Int32
}

func (g *gatedHealth) CheckHealth(ctx context.Context) error {
	g.entered.Add(1)
	if g.gate != nil {
		select {
		case <-g.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return g.flippingHealth.CheckHealth(ctx)
}

func TestHealthServesACachedFailureWithoutBlockingAndRefreshesItInTheBackground(t *testing.T) {
	ds := &gatedHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, clock := registryWithClock(t, ds)
	if first := registry.Health(everything(t))[0]; first.Healthy {
		t.Fatalf("first status = %+v", first)
	}
	checks := ds.calls.Load()

	var recovered *error
	ds.err.Store(recovered)
	ds.gate = make(chan struct{})
	clock.advance(3 * time.Second)

	returned := make(chan datasource.Status, 1)
	go func() { returned <- registry.Health(everything(t))[0] }()
	select {
	case got := <-returned:
		if got.Healthy {
			t.Fatalf("status = %+v, want the cached failure", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Health blocked on the refresh of a cached failure")
	}

	close(ds.gate)
	deadline := time.Now().Add(5 * time.Second)
	for !registry.Health(everything(t))[0].Healthy {
		if time.Now().After(deadline) {
			t.Fatalf("the background refresh never recovered the status; checks %d -> %d", checks, ds.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHealthDoesNotProbeACachedFailureInsideTheRecheckInterval(t *testing.T) {
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, clock := registryWithClock(t, ds)
	registry.Health(everything(t))
	clock.advance(time.Second)
	registry.Health(everything(t))
	if ds.calls.Load() != 1 {
		t.Fatalf("checks = %d, want 1", ds.calls.Load())
	}
}

func TestAProbeThatStartedBeforeAnAnswerDoesNotOverwriteIt(t *testing.T) {
	ds := &gatedHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, clock := registryWithClock(t, ds)
	registry.Health(everything(t))

	ds.gate = make(chan struct{})
	clock.advance(3 * time.Second)
	started := ds.entered.Load()
	recheck := make(chan datasource.Status, 1)
	go func() {
		got, _ := registry.Recheck(everything(t), "loki")
		recheck <- got
	}()
	for ds.entered.Load() == started {
		time.Sleep(time.Millisecond)
	}
	clock.advance(time.Second)
	registry.Answered("loki")
	close(ds.gate)

	if got := <-recheck; !got.Healthy {
		t.Fatalf("the late failure was returned: %+v", got)
	}
	if got := registry.Health(everything(t))[0]; !got.Healthy {
		t.Fatalf("the late failure overwrote the answer: %+v", got)
	}
}

type recordingHandler struct {
	slog.Handler
	warnings atomic.Int32
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.warnings.Add(1)
	}
	return nil
}

func TestAFailingProbeLogsOnceWhileQueriesKeepAnsweringBetweenProbes(t *testing.T) {
	handler := &recordingHandler{}
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, err := datasource.NewRegistry(slog.New(handler), datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeClock()
	registry.SetClock(clock.now)
	for range 4 {
		registry.Health(everything(t))
		registry.Answered("loki")
		clock.advance(11 * time.Second)
		registry.Health(everything(t))
	}
	if got := handler.warnings.Load(); got != 1 {
		t.Fatalf("warnings = %d, want 1", got)
	}
}

type traceReader struct {
	failingHealth
}

func (traceReader) Type() string { return "tempo" }
func (traceReader) Trace(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
	return datasource.Frame{}, nil
}

type spanReader struct {
	traceReader
}

func (spanReader) Type() string       { return "elasticsearch" }
func (spanReader) SpanFormat() string { return "jaeger" }

func TestStatusReportsTraceSearchOnlyWhereTheQueryPathAnswersIt(t *testing.T) {
	registry, err := datasource.NewRegistry(nil,
		datasource.Instance{UID: "tempo", Datasource: traceReader{}},
		datasource.Instance{UID: "spans", Datasource: spanReader{}},
		datasource.Instance{UID: "prom", Datasource: failingHealth{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"tempo": true, "spans": false, "prom": false}
	for _, s := range registry.Health(everything(t)) {
		if s.TraceSearch != want[s.UID] {
			t.Errorf("%s TraceSearch = %v, want %v", s.UID, s.TraceSearch, want[s.UID])
		}
	}
}

func TestACancelledCheckIsNeitherCachedNorAFailureOfTheDatasource(t *testing.T) {
	handler := &recordingHandler{}
	ds := &flippingHealth{}
	cancelled := fmt.Errorf("probe: %w", context.Canceled)
	ds.err.Store(&cancelled)
	registry, err := datasource.NewRegistry(slog.New(handler), datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	registry.SetClock(newFakeClock().now)

	got := registry.Health(everything(t))[0]
	if got.Healthy || got.Message != "The health check was cancelled" {
		t.Fatalf("status of a cancelled check = %+v", got)
	}
	if got.HealthState != datasource.HealthUnchecked || got.FailureKind != "" {
		t.Errorf("a cancelled check reads as a failure of the datasource: state %q, kind %q", got.HealthState, got.FailureKind)
	}
	if warnings := handler.warnings.Load(); warnings != 0 {
		t.Errorf("a cancelled check logged %d unhealthy warnings", warnings)
	}

	var answering *error
	ds.err.Store(answering)
	next := registry.Health(everything(t))[0]
	if !next.Healthy || next.HealthState != datasource.HealthHealthy || ds.calls.Load() != 2 {
		t.Fatalf("the next read served the cancelled check instead of checking again: %+v after %d checks", next, ds.calls.Load())
	}
}

func TestACancelledCheckLeavesTheCachedFailureInPlace(t *testing.T) {
	ds := &flippingHealth{}
	wrongType := datasource.Errorf(datasource.ErrIsOpenSearch, "wrong setup")
	ds.err.Store(&wrongType)
	registry, clock := registryWithClock(t, ds)
	failing := registry.Health(everything(t))[0]

	cancelled := context.Canceled
	ds.err.Store(&cancelled)
	clock.advance(3 * time.Second)
	rechecked, _ := registry.Recheck(everything(t), "loki")
	if rechecked.HealthState != datasource.HealthUnchecked || rechecked.FailureKind != "" {
		t.Fatalf("the cancelled recheck = %+v", rechecked)
	}
	served := registry.Health(everything(t))[0]
	if served.Message != failing.Message || served.FailureKind != datasource.FailureConfiguration || !served.CheckedAt.Equal(failing.CheckedAt) {
		t.Fatalf("status after a cancelled recheck = %+v, want the failure %+v", served, failing)
	}
}

func TestACallerThatLeavesGetsAnUncheckedStatus(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: blockedHealth{release: release}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(everything(t))
	cancel()
	got := registry.Health(ctx)[0]
	if got.HealthState != datasource.HealthUnchecked || got.FailureKind != "" || got.Healthy {
		t.Fatalf("status = %+v", got)
	}
}

func TestHealthStateFollowsTheLastCheckAndTheBackgroundRecheck(t *testing.T) {
	ds := &gatedHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, clock := registryWithClock(t, ds)

	failing := registry.Health(everything(t))[0]
	if failing.HealthState != datasource.HealthFailing || failing.FailureKind != datasource.FailureTransient {
		t.Fatalf("first status = %+v", failing)
	}
	clock.advance(time.Second)
	if inside := registry.Health(everything(t))[0]; inside.HealthState != datasource.HealthFailing {
		t.Fatalf("status inside the recheck interval = %+v", inside)
	}

	ds.gate = make(chan struct{})
	clock.advance(2 * time.Second)
	rechecking := registry.Health(everything(t))[0]
	if rechecking.HealthState != datasource.HealthRechecking || rechecking.Healthy ||
		rechecking.FailureKind != failing.FailureKind || rechecking.Message != failing.Message || rechecking.Detail != failing.Detail {
		t.Fatalf("status while the background check runs = %+v, want the failure %+v as rechecking", rechecking, failing)
	}

	var recovered *error
	ds.err.Store(recovered)
	close(ds.gate)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := registry.Health(everything(t))[0]
		if got.Healthy {
			if got.HealthState != datasource.HealthHealthy || got.FailureKind != "" {
				t.Fatalf("recovered status = %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background check never recovered the status")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAnAnsweredQueryMakesTheStateHealthy(t *testing.T) {
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrTimeout, "busy")
	ds.err.Store(&down)
	registry, _ := registryWithClock(t, ds)
	registry.Health(everything(t))
	registry.Answered("loki")
	if got := registry.Health(everything(t))[0]; got.HealthState != datasource.HealthHealthy || got.FailureKind != "" {
		t.Fatalf("status = %+v", got)
	}
}

func TestAFailedCheckNamesItsKind(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want datasource.FailureKind
	}{
		{"timeout", datasource.ErrTimeout, datasource.FailureTransient},
		{"deadline", context.DeadlineExceeded, datasource.FailureTransient},
		{"unavailable", datasource.ErrUnavailable, datasource.FailureTransient},
		{"limit", datasource.ErrLimitExceeded, datasource.FailureTransient},
		{"unclassified", errors.New("dial tcp: connection refused"), datasource.FailureTransient},
		{"auth failed", datasource.ErrAuthFailed, datasource.FailureCredentials},
		{"unreadable credentials", datasource.ErrUnreadable, datasource.FailureCredentials},
		{"host not allowed", datasource.ErrHostNotAllowed, datasource.FailureConfiguration},
		{"no secret key", datasource.ErrNoSecretKey, datasource.FailureConfiguration},
		{"not loadable", datasource.ErrNotLoadable, datasource.FailureConfiguration},
		{"outbound blocked", datasource.ErrOutboundBlocked, datasource.FailureConfiguration},
		{"not read-only", datasource.ErrNotReadOnly, datasource.FailureConfiguration},
		{"unsafe grant", datasource.ErrUnsafeGrant, datasource.FailureConfiguration},
		{"forbidden", datasource.ErrForbiddenQuery, datasource.FailureConfiguration},
		{"is opensearch", datasource.ErrIsOpenSearch, datasource.FailureConfiguration},
		{"is elasticsearch", datasource.ErrIsElasticsearch, datasource.FailureConfiguration},
		{"no index", datasource.ErrNoIndex, datasource.FailureConfiguration},
		{"no time field", datasource.ErrNoTimeField, datasource.FailureConfiguration},
		{"no role field", datasource.ErrNoRoleField, datasource.FailureConfiguration},
		{"invalid", datasource.ErrInvalidQuery, datasource.FailureConfiguration},
		{"unsupported version", datasource.ErrUnsupportedVersion, datasource.FailureUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: failingHealth{err: tt.err}})
			if err != nil {
				t.Fatal(err)
			}
			got := registry.Health(everything(t))[0]
			if got.HealthState != datasource.HealthFailing || got.FailureKind != tt.want {
				t.Errorf("state %q, kind %q, want failing, %q", got.HealthState, got.FailureKind, tt.want)
			}
			if !slices.Contains(datasource.FailureKinds, got.FailureKind) {
				t.Errorf("kind %q is not a declared failure kind", got.FailureKind)
			}
			if _, _, kind := datasource.Reason(tt.err); kind != tt.want {
				t.Errorf("Reason kind = %q, want %q", kind, tt.want)
			}
		})
	}
}

func TestAHealthyDatasourceHasNoFailureKindInItsJSON(t *testing.T) {
	registry, err := datasource.NewRegistry(nil,
		datasource.Instance{UID: "down", Datasource: failingHealth{err: datasource.ErrAuthFailed}},
		datasource.Instance{UID: "up", Datasource: failingHealth{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range registry.Health(everything(t)) {
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		_, hasKind := m["failure_kind"]
		switch s.UID {
		case "up":
			if m["health_state"] != "healthy" || hasKind {
				t.Errorf("healthy status = %s", raw)
			}
		case "down":
			if m["health_state"] != "failing" || m["failure_kind"] != "credentials" || m["healthy"] != false {
				t.Errorf("failing status = %s", raw)
			}
		}
	}
}
