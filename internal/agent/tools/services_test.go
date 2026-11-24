package tools

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type fakeServiceDS struct {
	*fakeDS
	field    string
	values   []string
	failWith error
	calls    atomic.Int32
}

func (f *fakeServiceDS) Services(_ context.Context, req datasource.ServiceRequest) (datasource.ServiceList, error) {
	f.calls.Add(1)
	if f.failWith != nil {
		return datasource.ServiceList{}, f.failWith
	}
	signal, err := datasource.ResolveServiceSignal(f.Type(), req.Signal)
	if err != nil {
		return datasource.ServiceList{}, err
	}
	return datasource.ServiceList{Signal: signal, Field: f.field, Values: f.values}, nil
}

func newServicesRegistry(t *testing.T, instances ...datasource.Instance) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, instances...)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
}

func servicesOf(t *testing.T, res Result) servicesOut {
	t.Helper()
	if res.IsError {
		t.Fatalf("res = %#v", res.Content)
	}
	return res.Content.(servicesOut)
}

func twoSources() (*fakeServiceDS, *fakeServiceDS) {
	loki := &fakeServiceDS{fakeDS: &fakeDS{typ: "loki"}, field: "service_name", values: []string{"checkout", "shop"}}
	tempo := &fakeServiceDS{fakeDS: &fakeDS{typ: "tempo"}, field: "resource.service.name", values: []string{"frontend", "Checkout-API"}}
	return loki, tempo
}

func TestListServicesAsksEverySourceThatHoldsOneAndSkipsTheRest(t *testing.T) {
	loki, tempo := twoSources()
	r := newServicesRegistry(t,
		datasource.Instance{UID: "loki", Datasource: loki},
		datasource.Instance{UID: "tempo", Datasource: tempo},
		datasource.Instance{UID: "prom", Datasource: &fakeDS{}},
	)
	res, err := call(t, r, nameListServices, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	out := servicesOf(t, res)
	if out.WindowSeconds != 3600 || len(out.Sources) != 2 {
		t.Fatalf("out = %+v, want two sources over 3600 seconds", out)
	}
	bySource := map[string]serviceSourceOut{}
	for _, s := range out.Sources {
		bySource[s.Datasource] = s
	}
	if got := bySource["loki"]; got.Signal != "logs" || got.Field != "service_name" || len(got.Services) != 2 {
		t.Fatalf("loki = %+v", got)
	}
	if got := bySource["tempo"]; got.Signal != "traces" || got.Field != "resource.service.name" {
		t.Fatalf("tempo = %+v", got)
	}
}

func TestListServicesFindsWhichSourcesHoldAServiceByAnyCase(t *testing.T) {
	loki, tempo := twoSources()
	r := newServicesRegistry(t,
		datasource.Instance{UID: "loki", Datasource: loki},
		datasource.Instance{UID: "tempo", Datasource: tempo},
	)
	res, err := call(t, r, nameListServices, `{"service":"CHECKOUT"}`)
	if err != nil {
		t.Fatal(err)
	}
	out := servicesOf(t, res)
	if len(out.Sources) != 2 || len(out.Sources[0].Services) != 1 || len(out.Sources[1].Services) != 1 {
		t.Fatalf("out = %+v, want each source reduced to its checkout match", out)
	}
	res, err = call(t, r, nameListServices, `{"service":"payments"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out := servicesOf(t, res); len(out.Sources) != 0 {
		t.Fatalf("out = %+v, want no source", out)
	}
}

func TestListServicesSaysWhichSourceIsUnavailableAndStillAnswersTheOthers(t *testing.T) {
	loki, tempo := twoSources()
	tempo.failWith = datasource.Errorf(datasource.ErrTimeout, "timed out")
	r := newServicesRegistry(t,
		datasource.Instance{UID: "loki", Datasource: loki},
		datasource.Instance{UID: "tempo", Datasource: tempo},
	)
	res, err := call(t, r, nameListServices, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	out := servicesOf(t, res)
	bySource := map[string]serviceSourceOut{}
	for _, s := range out.Sources {
		bySource[s.Datasource] = s
	}
	if bySource["tempo"].Unavailable != "query_timeout" || len(bySource["loki"].Services) != 2 {
		t.Fatalf("out = %+v", out)
	}
}

func TestListServicesWithOneDatasourceReportsItsFailureAndItsMismatches(t *testing.T) {
	loki, _ := twoSources()
	loki.failWith = datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	r := newServicesRegistry(t,
		datasource.Instance{UID: "loki", Datasource: loki},
		datasource.Instance{UID: "prom", Datasource: &fakeDS{}},
	)
	res, err := call(t, r, nameListServices, `{"datasource":"loki"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "datasource_unavailable" {
		t.Fatalf("code = %q", got)
	}
	res, err = call(t, r, nameListServices, `{"datasource":"prom"}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeInvalidRequest || !strings.Contains(res.Content.(errorBody).Message, "prom is a prometheus datasource") {
		t.Fatalf("res = %#v", res.Content)
	}
	res, err = call(t, r, nameListServices, `{"datasource":"loki","signal":"traces"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("res = %#v, want an error for traces on Loki", res.Content)
	}
}

func TestListServicesAsksClickHouseForBothSignalsAndOneWhenTold(t *testing.T) {
	ch := &fakeServiceDS{fakeDS: &fakeDS{typ: "clickhouse"}, field: "ServiceName", values: []string{"shop"}}
	r := newServicesRegistry(t, datasource.Instance{UID: "ch", Datasource: ch})
	res, err := call(t, r, nameListServices, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if out := servicesOf(t, res); len(out.Sources) != 2 || out.Sources[0].Signal != "logs" || out.Sources[1].Signal != "traces" {
		t.Fatalf("out = %+v, want logs then traces", out)
	}
	res, err = call(t, r, nameListServices, `{"signal":"traces","window":"15m"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out := servicesOf(t, res); len(out.Sources) != 1 || out.Sources[0].Signal != "traces" || out.WindowSeconds != 900 {
		t.Fatalf("out = %+v", out)
	}
}

func TestListServicesRejectsInvalidInput(t *testing.T) {
	loki, _ := twoSources()
	r := newServicesRegistry(t, datasource.Instance{UID: "loki", Datasource: loki})
	for name, input := range map[string]string{
		"unknown field":   `{"bogus":1}`,
		"bad signal":      `{"signal":"metrics"}`,
		"bad window":      `{"window":"soon"}`,
		"window too long": `{"window":"48h"}`,
		"unknown source":  `{"datasource":"nope"}`,
		"long service":    `{"service":"` + strings.Repeat("x", 201) + `"}`,
	} {
		res, err := call(t, r, nameListServices, input)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !res.IsError {
			t.Errorf("%s: res = %#v, want an error", name, res.Content)
		}
	}
	if got := loki.calls.Load(); got != 0 {
		t.Fatalf("source asked %d times for refused input", got)
	}
}

func TestListServicesSkipsAdminsOnlySourcesUntilOneIsNamed(t *testing.T) {
	loki, tempo := twoSources()
	r := newServicesRegistry(t,
		datasource.Instance{UID: "loki", Datasource: loki},
		datasource.Instance{UID: "tempo", Datasource: tempo, Audience: datasource.AudienceAdmins},
	)
	admin := auth.Principal{Role: auth.RoleAdmin}
	noted, read := datasource.NotingRestrictedReads(datasource.WithAccess(auth.WithPrincipal(context.Background(), admin), datasource.Access{By: admin}))
	res, err := r.Call(noted, nameListServices, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	out := servicesOf(t, res)
	if len(out.Sources) != 1 || out.Sources[0].Datasource != "loki" || tempo.calls.Load() != 0 || read() {
		t.Fatalf("out = %+v, tempo calls = %d, read = %t; want only loki read and the run unmarked", out, tempo.calls.Load(), read())
	}
	res, err = r.Call(noted, nameListServices, []byte(`{"datasource":"tempo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out := servicesOf(t, res); len(out.Sources) != 1 || out.Sources[0].Datasource != "tempo" || !read() {
		t.Fatalf("out = %+v, read = %t; want the named Admins-only source read and the run marked", out, read())
	}
}
