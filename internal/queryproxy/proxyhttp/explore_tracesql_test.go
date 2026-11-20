package proxyhttp

import (
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const traceSQL = "SELECT TraceId, ServiceName FROM otel_traces WHERE $__timeFilter(Timestamp)"

func newClickHouseHarness(t *testing.T) (*harness, *pageSource) {
	t.Helper()
	hn := newHarness(t)
	traces := &pageSource{typedDatasource: &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.ClickHouse}, byOffset: true}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "ch", Datasource: traces},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, traces
}

func traceSQLBody(size int) map[string]any {
	body := instantBody(traceSQL)
	body["page_size"] = size
	return body
}

func TestExploreRunsTraceSQLAsAPageUnderTheDraftLimits(t *testing.T) {
	hn, traces := newClickHouseHarness(t)
	traces.onPage = func(req datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
		return logFrames(), datasource.PageInfo{Page: req.Page, PageSize: req.PageSize, Total: 3, TotalRelation: datasource.TotalExact, Plain: true}, nil
	}
	for _, kind := range []auth.PrincipalKind{auth.KindUser, auth.KindServiceAccount} {
		rec := hn.explore(kind, "ch", traceSQLBody(50))
		if got, present := rawPage(t, rec)["plain"]; !present || string(got) != "true" {
			t.Fatalf("%s: meta.page.plain = %s, want true", kind, got)
		}
	}
	reqs := traces.pageRequests()
	if len(reqs) != 2 {
		t.Fatalf("page requests = %+v, want one per call", reqs)
	}
	for _, req := range reqs {
		if req.Query.Text != traceSQL || req.Query.Limits != queryproxy.DraftLimits || req.Page != 1 || req.PageSize != 50 || req.Sort != nil {
			t.Fatalf("page request = %+v, want the SQL as sent, page 1 of 50, draft limits and no sort", req)
		}
	}
}

func TestExploreAnswersTheTraceSQLRefusalWithItsCode(t *testing.T) {
	cases := []struct {
		name   string
		kind   error
		reason string
		code   string
	}{
		{"forbidden", datasource.ErrForbiddenQuery, "SETTINGS clauses are not allowed", "query_forbidden"},
		{"no range", datasource.ErrInvalidQuery, "a query without Timestamp must read the time range, for example WHERE $__timeFilter(EventTime)", "query_invalid"},
		{"too many rows", datasource.ErrLimitExceeded, "result exceeds 50 rows", "limit_exceeded"},
		{"database message", datasource.ErrInvalidQuery, "Syntax error: failed at position 8 (FROM)", "query_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hn, traces := newClickHouseHarness(t)
			traces.onPage = func(datasource.PageRequest) ([]datasource.Frame, datasource.PageInfo, error) {
				return nil, datasource.PageInfo{}, datasource.Errorf(tc.kind, "%s", tc.reason)
			}
			rec := hn.explore(auth.KindUser, "ch", traceSQLBody(50))
			expectError(t, rec, http.StatusUnprocessableEntity, tc.code)
			if !strings.Contains(rec.Body.String(), tc.reason) {
				t.Fatalf("body = %s, want the reason", rec.Body.String())
			}
		})
	}
}
