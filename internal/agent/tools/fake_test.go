package tools

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const fakeUID = "fake"

type fakeDS struct {
	typ        string
	rangeFn    func(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error)
	instantFn  func(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error)
	discoverFn func(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error)
	lintFn     func(ctx context.Context, query string) ([]datasource.Diagnostic, error)

	mu       sync.Mutex
	requests []datasource.DiscoverRequest
	steps    []time.Duration
}

func (f *fakeDS) Type() string {
	if f.typ == "" {
		return "prometheus"
	}
	return f.typ
}
func (f *fakeDS) CheckHealth(context.Context) error { return nil }
func (f *fakeDS) Lint(ctx context.Context, query string) ([]datasource.Diagnostic, error) {
	if f.lintFn != nil {
		return f.lintFn(ctx, query)
	}
	return nil, nil
}
func (f *fakeDS) Close() error { return nil }
func (f *fakeDS) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, nil
}

func (f *fakeDS) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.discoverFn != nil {
		return f.discoverFn(ctx, req)
	}
	return &datasource.Catalog{}, nil
}

func (f *fakeDS) QueryRange(ctx context.Context, q datasource.Query, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
	f.mu.Lock()
	f.steps = append(f.steps, step)
	f.mu.Unlock()
	if f.rangeFn != nil {
		return f.rangeFn(ctx, q, r, step)
	}
	return nil, nil
}

func (f *fakeDS) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	if f.instantFn != nil {
		return f.instantFn(ctx, q, r)
	}
	return nil, nil
}

func (f *fakeDS) lastRequest() datasource.DiscoverRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

func newTestRegistry(t *testing.T, ds *fakeDS, logger *slog.Logger) *Registry {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func call(t *testing.T, r *Registry, name, input string) (Result, error) {
	t.Helper()
	return r.Call(auth.WithPrincipal(context.Background(), auth.Principal{Role: auth.RoleAdmin}), name, []byte(input))
}

func errorCode(t *testing.T, res Result) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected a tool error, got %#v", res.Content)
	}
	body, ok := res.Content.(errorBody)
	if !ok {
		t.Fatalf("tool error content is %T, want errorBody", res.Content)
	}
	return body.Code
}

func timeNumberFrame(labels map[string]string, values ...float64) datasource.Frame {
	times := make([]any, len(values))
	nums := make([]any, len(values))
	for i, v := range values {
		times[i] = time.Unix(int64(i)*60, 0).UTC()
		nums[i] = v
	}
	return datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: nums},
	}}
}

func logTableFrame() datasource.Frame {
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	return datasource.Frame{Fields: []datasource.Field{
		{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{now, now.Add(time.Second)}},
		{Name: "ServiceName", Type: datasource.FieldString, Values: []any{"checkout", "cart"}},
		{Name: "Body", Type: datasource.FieldString, Values: []any{"payment failed", "item added"}},
	}}
}
