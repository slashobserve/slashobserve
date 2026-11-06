package elasticsearch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const (
	esRoot = `{"name":"es","cluster_name":"docker-cluster","version":{"number":"8.19.4","build_flavor":"default"},"tagline":"You Know, for Search"}`
	osRoot = `{"name":"os","cluster_name":"docker-cluster","version":{"distribution":"opensearch","number":"2.19.1","build_type":"tar"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`

	resolved = `{"indices":[{"name":"logs-shop-2026.11","attributes":["open"]},{"name":"logs-legacy-2026.11","attributes":["open"]}],"aliases":[{"name":"logs","indices":["logs-shop-2026.11"]}],"data_streams":[{"name":"logs-generic-default","backing_indices":[".ds-logs-generic-default-000001"],"timestamp_field":"@timestamp"}]}`
	noIndex  = `{"indices":[],"aliases":[],"data_streams":[]}`
)

type request struct {
	method string
	path   string
	params url.Values
	header http.Header
	body   string
}

type reply struct {
	status int
	body   string
}

type fakeServer struct {
	mu       sync.Mutex
	routes   map[string]func(request) reply
	requests []request
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := request{method: r.Method, path: r.URL.Path, params: r.URL.Query(), header: r.Header.Clone(), body: string(body)}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	handler, ok := f.routes[r.Method+" "+r.URL.Path]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"root_cause":[{"type":"index_not_found_exception","reason":"no such index"}],"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
		return
	}
	out := handler(req)
	if out.status == 0 {
		out.status = http.StatusOK
	}
	w.WriteHeader(out.status)
	_, _ = w.Write([]byte(out.body))
}

func (f *fakeServer) set(method, path string, handler func(request) reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = handler
}

func (f *fakeServer) fixed(method, path string, status int, body string) {
	f.set(method, path, func(request) reply { return reply{status: status, body: body} })
}

func (f *fakeServer) last(t *testing.T) request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the fake server")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeServer) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.path == path {
			n++
		}
	}
	return n
}

func fieldCaps(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/field_caps.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func healthyServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{routes: map[string]func(request) reply{}}
	f.fixed(http.MethodGet, "/", http.StatusOK, esRoot)
	f.fixed(http.MethodGet, "/_resolve/index/logs-*", http.StatusOK, resolved)
	f.fixed(http.MethodPost, "/logs-*/_field_caps", http.StatusOK, fieldCaps(t))
	return f
}

func serve(t *testing.T, f *fakeServer, mutate ...func(*elasticsearch.Options)) *elasticsearch.Datasource {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	opts := elasticsearch.Options{Flavor: elasticsearch.Elasticsearch, URL: srv.URL, Index: "logs-*"}
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := elasticsearch.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func esError(typ, reason string) string {
	return fmt.Sprintf(`{"error":{"root_cause":[{"type":%q,"reason":%q}],"type":%q,"reason":%q},"status":400}`, typ, reason, typ, reason)
}

func TestNewValidatesOptions(t *testing.T) {
	base := elasticsearch.Options{Flavor: elasticsearch.Elasticsearch, URL: "http://es:9200", Index: "logs-*"}
	with := func(m func(*elasticsearch.Options)) elasticsearch.Options {
		o := base
		m(&o)
		return o
	}
	tests := []struct {
		name string
		opts elasticsearch.Options
		want string
	}{
		{"flavor", with(func(o *elasticsearch.Options) { o.Flavor = "elastic" }), "flavor must be elasticsearch or opensearch"},
		{"url scheme", with(func(o *elasticsearch.Options) { o.URL = "ftp://es" }), "url scheme must be http or https"},
		{"url user info", with(func(o *elasticsearch.Options) { o.URL = "http://u:p@es:9200" }), "url must not contain user info"},
		{"url query", with(func(o *elasticsearch.Options) { o.URL = "http://es:9200/?a=b" }), "url must not contain a query or fragment"},
		{"index missing", with(func(o *elasticsearch.Options) { o.Index = "" }), "index is required"},
		{"index whitespace", with(func(o *elasticsearch.Options) { o.Index = "logs *" }), "index must not contain whitespace"},
		{"query language", with(func(o *elasticsearch.Options) { o.QueryLanguage = "sql" }), "query language must be kql or lucene"},
		{"negative max bytes", with(func(o *elasticsearch.Options) { o.MaxResponseBytes = -1 }), "max response bytes must not be negative"},
		{"password without user", with(func(o *elasticsearch.Options) { o.BasicAuthPassword = datasource.NewSecret("p") }), "basic auth password requires a user"},
		{"basic and bearer", with(func(o *elasticsearch.Options) {
			o.BasicAuthUser, o.BasicAuthPassword, o.BearerToken = "u", datasource.NewSecret("p"), datasource.NewSecret("t")
		}), "mutually exclusive"},
		{"bearer and api key", with(func(o *elasticsearch.Options) {
			o.BearerToken, o.APIKey = datasource.NewSecret("t"), datasource.NewSecret("k")
		}), "mutually exclusive"},
		{"authorization header", with(func(o *elasticsearch.Options) { o.Headers = map[string]string{"authorization": "x"} }), "the Authorization header is set through"},
		{"reserved header", with(func(o *elasticsearch.Options) { o.Headers = map[string]string{"content-type": "x"} }), "the Content-Type header is managed"},
		{"invalid header", with(func(o *elasticsearch.Options) { o.Headers = map[string]string{"X A": "x"} }), "invalid header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := elasticsearch.New(tt.opts)
			var cfg *datasource.ConfigError
			if !errors.As(err, &cfg) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want a config error containing %q", err, tt.want)
			}
			for _, secret := range []string{"p", "t", "k"} {
				if strings.Contains(err.Error(), `"`+secret+`"`) {
					t.Fatalf("error %q echoes a secret", err)
				}
			}
		})
	}
	ds, err := elasticsearch.New(with(func(o *elasticsearch.Options) { o.Flavor = elasticsearch.OpenSearch }))
	if err != nil || ds.Type() != "opensearch" {
		t.Fatalf("New(opensearch) = %v, %v", ds, err)
	}
}

func TestRequestsCarryAcceptHeadersAndCredentials(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*elasticsearch.Options)
		want   string
	}{
		{"api key", func(o *elasticsearch.Options) { o.APIKey = datasource.NewSecret("secret-key") }, "ApiKey secret-key"},
		{"bearer", func(o *elasticsearch.Options) { o.BearerToken = datasource.NewSecret("tok") }, "Bearer tok"},
		{"basic", func(o *elasticsearch.Options) { o.BasicAuthUser, o.BasicAuthPassword = "u", datasource.NewSecret("p") }, "Basic dTpw"},
		{"none", func(*elasticsearch.Options) {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthyServer(t)
			ds := serve(t, f, tt.mutate, func(o *elasticsearch.Options) { o.Headers = map[string]string{"X-Team": "shop"} })
			if err := ds.CheckHealth(context.Background()); err != nil {
				t.Fatalf("CheckHealth: %v", err)
			}
			req := f.last(t)
			if req.header.Get("Authorization") != tt.want || req.header.Get("Accept") != "application/json" || req.header.Get("X-Team") != "shop" {
				t.Fatalf("headers = %v", req.header)
			}
			if req.method == http.MethodPost && req.header.Get("Content-Type") != "application/json" {
				t.Fatalf("POST without a JSON content type: %v", req.header)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeServer)
		flavor elasticsearch.Flavor
		want   error
	}{
		{"elasticsearch healthy", func(*fakeServer) {}, elasticsearch.Elasticsearch, nil},
		{"opensearch healthy", func(f *fakeServer) { f.fixed(http.MethodGet, "/", 200, osRoot) }, elasticsearch.OpenSearch, nil},
		{"opensearch server configured as elasticsearch", func(f *fakeServer) { f.fixed(http.MethodGet, "/", 200, osRoot) }, elasticsearch.Elasticsearch, datasource.ErrIsOpenSearch},
		{"elasticsearch server configured as opensearch", func(*fakeServer) {}, elasticsearch.OpenSearch, datasource.ErrIsElasticsearch},
		{"elasticsearch 7.10", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/", 200, `{"version":{"number":"7.10.2"}}`)
		}, elasticsearch.Elasticsearch, datasource.ErrUnsupportedVersion},
		{"elasticsearch 10", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/", 200, `{"version":{"number":"10.0.0"}}`)
		}, elasticsearch.Elasticsearch, datasource.ErrUnsupportedVersion},
		{"opensearch 1.3", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/", 200, `{"version":{"distribution":"opensearch","number":"1.3.20"}}`)
		}, elasticsearch.OpenSearch, datasource.ErrUnsupportedVersion},
		{"opensearch 3", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/", 200, `{"version":{"distribution":"opensearch","number":"3.1.0"}}`)
		}, elasticsearch.OpenSearch, nil},
		{"unreadable root", func(f *fakeServer) { f.fixed(http.MethodGet, "/", 200, `<html>`) }, elasticsearch.Elasticsearch, datasource.ErrUnavailable},
		{"pattern matches nothing", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/_resolve/index/logs-*", 200, noIndex)
		}, elasticsearch.Elasticsearch, datasource.ErrNoIndex},
		{"concrete index missing", func(f *fakeServer) {
			f.fixed(http.MethodGet, "/_resolve/index/logs-*", 404, `{"error":{"root_cause":[{"type":"index_not_found_exception","reason":"no such index [logs-*]"}],"type":"index_not_found_exception","reason":"no such index [logs-*]"},"status":404}`)
		}, elasticsearch.Elasticsearch, datasource.ErrNoIndex},
		{"time field missing", func(f *fakeServer) {
			f.fixed(http.MethodPost, "/logs-*/_field_caps", 200, `{"indices":["a"],"fields":{"ts":{"keyword":{"type":"keyword","metadata_field":false,"searchable":true,"aggregatable":true}}}}`)
		}, elasticsearch.Elasticsearch, datasource.ErrNoTimeField},
		{"credentials refused", func(f *fakeServer) { f.fixed(http.MethodGet, "/", 401, `{"error":"unauthorized"}`) }, elasticsearch.Elasticsearch, datasource.ErrAuthFailed},
		{"server down", func(f *fakeServer) { f.fixed(http.MethodGet, "/", 503, ``) }, elasticsearch.Elasticsearch, datasource.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthyServer(t)
			tt.mutate(f)
			ds := serve(t, f, func(o *elasticsearch.Options) { o.Flavor = tt.flavor })
			err := ds.CheckHealth(context.Background())
			if tt.want == nil {
				if err != nil {
					t.Fatalf("CheckHealth: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("CheckHealth error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestHealthReportsAConfiguredRoleThatIsMissing(t *testing.T) {
	ds := serve(t, healthyServer(t), func(o *elasticsearch.Options) { o.Fields.Level = "severity" })
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrNoRoleField) {
		t.Fatalf("CheckHealth error = %v, want ErrNoRoleField", err)
	}
}

func TestHealthChecksTheRequestsItSends(t *testing.T) {
	f := healthyServer(t)
	ds := serve(t, f)
	if err := ds.CheckHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 3 {
		t.Fatalf("requests = %+v", f.requests)
	}
	resolve, caps := f.requests[1], f.requests[2]
	if resolve.params.Get("expand_wildcards") != "open" {
		t.Fatalf("resolve params = %v", resolve.params)
	}
	if caps.params.Get("fields") != "*" || caps.params.Get("ignore_unavailable") != "true" || caps.params.Get("allow_no_indices") != "true" {
		t.Fatalf("field caps params = %v", caps.params)
	}
	if caps.body != `{"index_filter":{"range":{"@timestamp":{"gte":"now-7d"}}}}` {
		t.Fatalf("field caps body = %s", caps.body)
	}
}

func TestStatusError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
		text   string
	}{
		{"forbidden", 403, `{"error":{"type":"security_exception","reason":"forbidden"}}`, datasource.ErrAuthFailed, "authentication failed (HTTP 403)"},
		{"index not found", 404, esError("index_not_found_exception", "no such index [logs-*]"), datasource.ErrNoIndex, "no such index [logs-*]"},
		{"parsing exception", 400, esError("parsing_exception", "[match] unknown token"), datasource.ErrInvalidQuery, "[match] unknown token"},
		{"search phase with a query shard root cause", 400, `{"error":{"root_cause":[{"type":"query_shard_exception","reason":"Failed to parse query [foo:]"}],"type":"search_phase_execution_exception","reason":"all shards failed"},"status":400}`, datasource.ErrInvalidQuery, "Failed to parse query [foo:]"},
		{"too many buckets", 400, esError("too_many_buckets_exception", "Trying to create too many buckets"), datasource.ErrLimitExceeded, "Trying to create too many buckets"},
		{"too many buckets as 500", 500, esError("too_many_buckets_exception", "Trying to create too many buckets"), datasource.ErrLimitExceeded, "Trying to create too many buckets"},
		{"circuit breaking", 429, esError("circuit_breaking_exception", "[parent] Data too large"), datasource.ErrLimitExceeded, "[parent] Data too large"},
		{"rejected execution", 429, esError("es_rejected_execution_exception", "rejected execution of search"), datasource.ErrUnavailable, "rate limited by upstream"},
		{"request timeout", 408, ``, datasource.ErrTimeout, "query timed out"},
		{"gateway timeout", 504, `upstream timed out`, datasource.ErrTimeout, "upstream timed out"},
		{"bad gateway", 502, `<html>bad gateway</html>`, datasource.ErrUnavailable, "<html>bad gateway</html>"},
		{"unavailable", 503, `{"error":{"type":"master_not_discovered_exception","reason":null}}`, datasource.ErrUnavailable, "upstream returned HTTP 503"},
		{"reason naming an address is replaced", 400, esError("illegal_argument_exception", "node 10.0.0.1:9200 refused"), datasource.ErrInvalidQuery, "query rejected by the datasource"},
		{"plain 400", 400, `{"error":{"type":"action_request_validation_exception","reason":"Validation Failed: 1: index is missing;"}}`, datasource.ErrInvalidQuery, "Validation Failed: 1: index is missing;"},
		{"string error", 405, `{"error":"Incorrect HTTP method for uri [/] and method [PUT]"}`, datasource.ErrUnavailable, "Incorrect HTTP method for uri [/] and method [PUT]"},
		{"long reason is capped", 400, esError("parsing_exception", strings.Repeat("x", 600)), datasource.ErrInvalidQuery, strings.Repeat("x", 512)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthyServer(t)
			f.fixed(http.MethodGet, "/", tt.status, tt.body)
			err := serve(t, f).CheckHealth(context.Background())
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			var up *datasource.UpstreamError
			if !errors.As(err, &up) || up.Message != tt.text {
				t.Fatalf("message = %q, want %q", err, tt.text)
			}
		})
	}
}

func TestOversizedResponseIsRefused(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodGet, "/", 200, strings.Repeat(" ", 2000)+esRoot)
	ds := serve(t, f, func(o *elasticsearch.Options) { o.MaxResponseBytes = 1024 })
	if err := ds.CheckHealth(context.Background()); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("error = %v, want ErrLimitExceeded", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	f := healthyServer(t)
	f.set(http.MethodGet, "/", func(request) reply { return reply{status: http.StatusFound, body: ""} })
	if err := serve(t, f).CheckHealth(context.Background()); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
