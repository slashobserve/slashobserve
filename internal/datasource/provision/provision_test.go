package provision

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func prom(uid, url string) config.Datasource {
	return config.Datasource{UID: uid, Type: "prometheus", Name: uid, Prometheus: &config.PrometheusDatasource{URL: url}}
}

func everything(t *testing.T) context.Context {
	return datasource.WithAccess(t.Context(), datasource.Access{System: true})
}

func TestBuildCopiesGrafanaUIDs(t *testing.T) {
	src := prom("prom", "http://prom:9090")
	src.GrafanaUIDs = []string{"prometheus", "Main"}
	registry, err := Build(nil, []config.Datasource{src, prom("other", "http://other:9090")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	in, err := registry.For(everything(t), "prom")
	if err != nil || !slices.Equal(in.GrafanaUIDs, []string{"prometheus", "Main"}) {
		t.Fatalf("instance = %+v", in)
	}
	if other, _ := registry.For(everything(t), "other"); len(other.GrafanaUIDs) != 0 {
		t.Fatalf("other = %+v", other)
	}
}

func TestRegistryHandsOutCopiesOfGrafanaUIDs(t *testing.T) {
	src := prom("prom", "http://prom:9090")
	src.GrafanaUIDs = []string{"prometheus"}
	registry, err := Build(nil, []config.Datasource{src})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	listed := registry.Visible(everything(t))
	listed[0].GrafanaUIDs[0] = "changed"
	got, _ := registry.For(everything(t), "prom")
	got.GrafanaUIDs[0] = "changed again"
	if again, _ := registry.For(everything(t), "prom"); !slices.Equal(again.GrafanaUIDs, []string{"prometheus"}) {
		t.Fatalf("registry changed: %q", again.GrafanaUIDs)
	}
	if again := registry.Visible(everything(t)); !slices.Equal(again[0].GrafanaUIDs, []string{"prometheus"}) {
		t.Fatalf("registry changed: %q", again[0].GrafanaUIDs)
	}
}

func TestRegistryIsNotChangedByMutatingItsInput(t *testing.T) {
	src := prom("prom", "http://prom:9090")
	src.GrafanaUIDs = []string{"prometheus"}
	sources := []config.Datasource{src}
	registry, err := Build(nil, sources)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	sources[0].GrafanaUIDs[0] = "changed"
	if got, _ := registry.For(everything(t), "prom"); !slices.Equal(got.GrafanaUIDs, []string{"prometheus"}) {
		t.Fatalf("registry changed: %q", got.GrafanaUIDs)
	}
}

func TestBuildEmpty(t *testing.T) {
	registry, err := Build(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Visible(everything(t))) != 0 {
		t.Fatal("expected an empty registry")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFailureAfterEarlierDatasourcesWereBuilt(t *testing.T) {
	sources := []config.Datasource{
		prom("first", "http://first:9090"),
		prom("second", "http://second:9090"),
		prom("bad", "ftp://nope"),
	}
	registry, err := Build(nil, sources)
	if registry != nil {
		t.Fatal("expected no registry")
	}
	if err == nil || !strings.HasPrefix(err.Error(), `datasource "bad": `) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDuplicateUIDClosesEverything(t *testing.T) {
	registry, err := Build(nil, []config.Datasource{prom("same", "http://a:9090"), prom("same", "http://b:9090")})
	if registry != nil || err == nil || !strings.Contains(err.Error(), "duplicate datasource uid") {
		t.Fatalf("registry=%v err=%v", registry, err)
	}
}

func TestBuildWithoutBlockFails(t *testing.T) {
	_, err := Build(nil, []config.Datasource{{UID: "x", Type: "prometheus"}})
	if err == nil || !strings.Contains(err.Error(), "no datasource block configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildSendsSecretAndPlainHeaders(t *testing.T) {
	var got http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer upstream.Close()

	src := prom("p", upstream.URL)
	src.Prometheus.Headers = map[string]string{"X-Plain": "plain"}
	src.Prometheus.SecretHeaders = map[string]datasource.Secret{"X-Api-Key": datasource.NewSecret("key-value")}
	registry, err := Build(nil, []config.Datasource{src})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registry.Close() }()

	in, _ := registry.For(everything(t), "p")
	_ = in.Datasource.CheckHealth(context.Background())
	if got.Get("X-Plain") != "plain" || got.Get("X-Api-Key") != "key-value" {
		t.Fatalf("upstream saw headers %v", got)
	}
}

func TestBuildRejectsReservedSecretHeaderWithoutEchoingIt(t *testing.T) {
	src := prom("p", "http://p:9090")
	src.Prometheus.SecretHeaders = map[string]datasource.Secret{"Authorization": datasource.NewSecret("hunter2-token")}
	_, err := Build(nil, []config.Datasource{src})
	if err == nil || strings.Contains(err.Error(), "hunter2-token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildLokiAndTempoSendTheirConnectionSettings(t *testing.T) {
	seen := map[string]http.Header{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Header.Get("X-Source")] = r.Header.Clone()
		if r.URL.Path == "/loki/api/v1/query" {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
			return
		}
		_, _ = w.Write([]byte("ready"))
	}))
	defer upstream.Close()
	logs := config.Datasource{UID: "logs", Type: "loki", Loki: &config.LokiDatasource{
		HTTPConnection: config.HTTPConnection{
			URL:           upstream.URL,
			Headers:       map[string]string{"X-Source": "loki"},
			SecretHeaders: map[string]datasource.Secret{"X-Api-Key": datasource.NewSecret("loki-key")},
			BearerToken:   datasource.NewSecret("loki-token"),
		},
		TenantID: "shop",
	}}
	traces := config.Datasource{UID: "traces", Type: "tempo", Tempo: &config.TempoDatasource{
		HTTPConnection: config.HTTPConnection{
			URL:               upstream.URL,
			Headers:           map[string]string{"X-Source": "tempo"},
			BasicAuthUser:     "reader",
			BasicAuthPassword: datasource.NewSecret("pw"),
		},
	}}
	jaegerTraces := config.Datasource{UID: "jaeger", Type: "jaeger", Jaeger: &config.JaegerDatasource{
		HTTPConnection: config.HTTPConnection{
			URL:         upstream.URL,
			Headers:     map[string]string{"X-Source": "jaeger"},
			BearerToken: datasource.NewSecret("jaeger-token"),
		},
	}}
	registry, err := Build(nil, []config.Datasource{logs, traces, jaegerTraces})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registry.Close() }()
	for uid, typ := range map[string]string{"logs": "loki", "traces": "tempo", "jaeger": "jaeger"} {
		in, _ := registry.For(everything(t), uid)
		if in.Datasource.Type() != typ {
			t.Fatalf("%s type = %s, want %s", uid, in.Datasource.Type(), typ)
		}
		if err := in.Datasource.CheckHealth(context.Background()); err != nil {
			t.Fatalf("%s health: %v", uid, err)
		}
	}
	if h := seen["loki"]; h.Get("X-Scope-OrgID") != "shop" || h.Get("X-Api-Key") != "loki-key" || h.Get("Authorization") != "Bearer loki-token" {
		t.Fatalf("loki sent %v", h)
	}
	if h := seen["tempo"]; h.Get("Authorization") != "Basic cmVhZGVyOnB3" {
		t.Fatalf("tempo sent %v", h)
	}
	if h := seen["jaeger"]; h.Get("Authorization") != "Bearer jaeger-token" {
		t.Fatalf("jaeger sent %v", h)
	}
}

func TestBuildLokiAndTempoRefuseInvalidSettings(t *testing.T) {
	for _, src := range []config.Datasource{
		{UID: "logs", Type: "loki", Loki: &config.LokiDatasource{HTTPConnection: config.HTTPConnection{URL: "ftp://loki"}}},
		{UID: "traces", Type: "tempo", Tempo: &config.TempoDatasource{HTTPConnection: config.HTTPConnection{URL: "ftp://tempo"}}},
		{UID: "jaeger", Type: "jaeger", Jaeger: &config.JaegerDatasource{HTTPConnection: config.HTTPConnection{URL: "ftp://jaeger"}}},
	} {
		_, err := Build(nil, []config.Datasource{src})
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || !strings.Contains(buildErr.Message, "url scheme must be http or https") {
			t.Fatalf("%s: err = %v", src.UID, err)
		}
	}
}

func TestBuildKeepsTheAudienceOfTheFile(t *testing.T) {
	audit := prom("audit", "http://audit:9090")
	audit.Audience = datasource.AudienceAdmins
	registry, err := Build(nil, []config.Datasource{prom("prom", "http://prom:9090"), audit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	if !registry.AdminsOnly("audit") || registry.AdminsOnly("prom") {
		t.Fatal("the file's audience did not reach the registry")
	}
	viewer := datasource.WithAccess(t.Context(), datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "v", Role: auth.RoleViewer}})
	if _, err := registry.For(viewer, "audit"); !errors.Is(err, datasource.ErrNotVisible) {
		t.Fatalf("viewer For(audit) err = %v", err)
	}
}
