package provision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func prom(uid, url string) config.Datasource {
	return config.Datasource{UID: uid, Type: "prometheus", Name: uid, Prometheus: &config.PrometheusDatasource{URL: url}}
}

func TestBuildEmpty(t *testing.T) {
	registry, err := Build(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.List()) != 0 {
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

	in, _ := registry.Get("p")
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
