package datasource_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

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
			statuses := registry.Health(t.Context())
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
	ctx, cancel := context.WithCancel(t.Context())
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
	for _, s := range registry.Health(t.Context()) {
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
	ctx, cancel := context.WithCancel(t.Context())
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
			got := registry.Health(t.Context())[0]
			if got.Detail != tt.want {
				t.Errorf("detail = %q, want %q", got.Detail, tt.want)
			}
			if strings.Contains(got.Detail, "10.1.2.3") {
				t.Errorf("detail %q carries the address", got.Detail)
			}
		})
	}
}

func TestRecheckRunsAFreshCheckAndThrottlesRepeats(t *testing.T) {
	ds := &flippingHealth{}
	down := datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	ds.err.Store(&down)
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	if first := registry.Health(t.Context())[0]; first.Healthy {
		t.Fatalf("first status = %+v", first)
	}

	var recovered *error
	ds.err.Store(recovered)
	if cached := registry.Health(t.Context())[0]; cached.Healthy {
		t.Fatalf("a plain health read inside the cache window must not recheck: %+v", cached)
	}
	time.Sleep(2100 * time.Millisecond)
	got, ok := registry.Recheck(t.Context(), "loki")
	if !ok || !got.Healthy || got.Detail != "" {
		t.Fatalf("recheck = %+v, ok %v", got, ok)
	}
	calls := ds.calls.Load()
	if again, _ := registry.Recheck(t.Context(), "loki"); !again.Healthy || ds.calls.Load() != calls {
		t.Errorf("a recheck inside the throttle window ran another check: calls %d -> %d", calls, ds.calls.Load())
	}
	if _, ok := registry.Recheck(t.Context(), "missing"); ok {
		t.Error("recheck of an unknown uid reported found")
	}
}
