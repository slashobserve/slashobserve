package proxyhttp

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/public"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type fakePublicLinks map[string]public.Resolved

func (f fakePublicLinks) Resolve(_ context.Context, token string) (public.Resolved, error) {
	if r, ok := f[token]; ok {
		return r, nil
	}
	return public.Resolved{}, public.ErrNotFound
}

func (hn *harness) publicRouter(links fakePublicLinks) *gin.Engine {
	router := gin.New()
	NewPublicData(hn.h, links).RegisterPublic(router.Group("/api/v1/public"))
	return router
}

func (hn *harness) publicLink(mode public.Mode, version int, stored map[string][]string) public.Resolved {
	return public.Resolved{LinkID: randomLinkID(), DashboardID: hn.id, Version: version, Mode: mode, Selections: stored}
}

func randomLinkID() pgtype.UUID {
	id := pgtype.UUID{Valid: true}
	_, _ = rand.Read(id.Bytes[:])
	return id
}

func ranWith(t *testing.T, q datasource.Query, name, want string) {
	t.Helper()
	for _, v := range q.Variables {
		if v.Name == name {
			if !slices.Equal(v.Values, []string{want}) {
				t.Fatalf("%s ran with %v, want %q", name, v.Values, want)
			}
			return
		}
	}
	t.Fatalf("%s was not passed to the query", name)
}

func publicGet(router *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPublicDataRefusals(t *testing.T) {
	hn := newHarness(t)
	scoped := testDashboard()
	scoped.Panels[0].Queries[0].Query = `up{owner="${__user.login}"}`
	hn.specs.known[specKey{id: hn.id, version: 2}] = scoped
	router := hn.publicRouter(fakePublicLinks{
		"ok":      hn.publicLink(public.ModeFrozen, 1, nil),
		"scoped":  hn.publicLink(public.ModeFrozen, 2, nil),
		"missing": hn.publicLink(public.ModeFrozen, 3, nil),
	})
	cases := []struct {
		name   string
		path   string
		status int
		code   string
	}{
		{"an unknown token", "/api/v1/public/nope/panels/p_ts/data?" + window, http.StatusNotFound, "not_found"},
		{"unknown token values", "/api/v1/public/nope/variables/job/values?" + window, http.StatusNotFound, "not_found"},
		{"a dashboard that reads the viewer", "/api/v1/public/scoped/panels/p_ts/data?" + window, http.StatusNotFound, "not_found"},
		{"a deleted dashboard", "/api/v1/public/missing/panels/p_ts/data?" + window, http.StatusNotFound, "not_found"},
		{"an unknown panel", "/api/v1/public/ok/panels/nope/data?" + window, http.StatusNotFound, "panel_not_found"},
		{"query text", "/api/v1/public/ok/panels/p_ts/data?query=up&" + window, http.StatusForbidden, "query_not_in_spec"},
		{"query text on values", "/api/v1/public/ok/variables/job/values?expr=up&" + window, http.StatusForbidden, "query_not_in_spec"},
		{"a bad time", "/api/v1/public/ok/panels/p_ts/data?from=soon", http.StatusBadRequest, "invalid_request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expectError(t, publicGet(router, c.path), c.status, c.code)
		})
	}
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held after the refusals", hn.heldSlots())
	}
}

func TestPublicDataFrozenIgnoresRequestedVariables(t *testing.T) {
	hn := newHarness(t)
	router := hn.publicRouter(fakePublicLinks{"tok": hn.publicLink(public.ModeFrozen, 1, map[string][]string{"job": {"web"}})})
	decodeData(t, publicGet(router, "/api/v1/public/tok/panels/p_ts/data?"+window+"&var-job=secret&var-env=dev"))
	ranWith(t, hn.ds.lastQuery(), "job", "web")
}

func TestPublicDataFreeAcceptsOnlyOptions(t *testing.T) {
	hn := newHarness(t)
	router := hn.publicRouter(fakePublicLinks{"tok": hn.publicLink(public.ModeFree, 1, map[string][]string{"job": {"api"}})})
	decodeData(t, publicGet(router, "/api/v1/public/tok/panels/p_ts/data?"+window+"&var-job=web&var-env=dev"))
	ranWith(t, hn.ds.lastQuery(), "job", "web")
	expectError(t, publicGet(router, "/api/v1/public/tok/panels/p_ts/data?"+window+"&var-job=api&var-job=web"), http.StatusBadRequest, "invalid_request")
	for _, query := range []string{"var-job=secret", "var-env=staging", "var-job=$__all"} {
		expectError(t, publicGet(router, "/api/v1/public/tok/panels/p_ts/data?"+window+"&"+query), http.StatusBadRequest, "selection_not_allowed")
	}
	expectError(t, publicGet(router, "/api/v1/public/tok/variables/env/values?"+window+"&var-job=secret"), http.StatusBadRequest, "selection_not_allowed")
	if hn.heldSlots() != 0 {
		t.Fatalf("%d slots held", hn.heldSlots())
	}
}

func TestPublicDataChecksFreeChoicesAgainstTheValuesCache(t *testing.T) {
	hn := newHarness(t)
	router := hn.publicRouter(fakePublicLinks{"tok": hn.publicLink(public.ModeFree, 1, nil)})
	if rec := publicGet(router, "/api/v1/public/tok/variables/job/values?"+window); rec.Code != http.StatusOK {
		t.Fatalf("values: %d %s", rec.Code, rec.Body.String())
	}
	_, _, before, _ := hn.ds.counts()
	decodeData(t, publicGet(router, "/api/v1/public/tok/panels/p_ts/data?"+window+"&var-job=web&var-env=prod"))
	if _, _, after, _ := hn.ds.counts(); after != before {
		t.Fatalf("the option check ran %d variable queries, want the cached values", after-before)
	}
}

func TestPublicDataTakesSlotsPerLink(t *testing.T) {
	hn := newHarness(t)
	link := hn.publicLink(public.ModeFrozen, 1, map[string][]string{"job": {"api"}})
	router := hn.publicRouter(fakePublicLinks{"tok": link})
	path := "/api/v1/public/tok/panels/p_tbl/data?" + window
	var releases []func()
	for range maxInFlightPerPrincipal {
		release, ok := hn.h.slots.acquire("alice")
		if !ok {
			t.Fatal("could not hold a principal slot")
		}
		releases = append(releases, release)
	}
	decodeData(t, publicGet(router, path))
	for range maxInFlightPerPrincipal {
		release, ok := hn.h.slots.acquire(publicSlot(link))
		if !ok {
			t.Fatal("could not hold a link slot")
		}
		releases = append(releases, release)
	}
	expectError(t, publicGet(router, path), http.StatusTooManyRequests, "too_many_requests")
	for _, release := range releases {
		release()
	}
	decodeData(t, publicGet(router, path))
	if publicSlot(link) != "public:"+link.LinkID.String() {
		t.Fatalf("slot key %q", publicSlot(link))
	}
}

func TestPublicValuesSeeOnlyEarlierSelections(t *testing.T) {
	hn := newHarness(t)
	d := testDashboard()
	d.Variables = append(d.Variables, spec.Variable{Name: "pod", Type: spec.QueryVariable, Datasource: "prom", Query: `label_values(up{job="$job"}, pod)`})
	hn.specs.known[specKey{id: hn.id, version: 1}] = d
	router := hn.publicRouter(fakePublicLinks{"tok": hn.publicLink(public.ModeFrozen, 1, map[string][]string{"job": {"web"}, "env": {"dev"}, "pod": {"a"}})})
	if rec := publicGet(router, "/api/v1/public/tok/variables/pod/values?"+window); rec.Code != http.StatusOK {
		t.Fatalf("values: %d %s", rec.Code, rec.Body.String())
	}
	ranWith(t, hn.ds.lastQuery(), "job", "web")
	if rec := publicGet(router, "/api/v1/public/tok/variables/job/values?"+window); rec.Code != http.StatusOK {
		t.Fatalf("first variable values: %d %s", rec.Code, rec.Body.String())
	}
}
