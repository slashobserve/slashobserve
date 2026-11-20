package proxyhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/public"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	auditVersion  = 2
	pickedVersion = 3
)

type reader struct {
	name  string
	kind  auth.PrincipalKind
	role  auth.Role
	reads bool
}

var readers = []reader{
	{"viewer", auth.KindUser, auth.RoleViewer, false},
	{"commenter", auth.KindUser, auth.RoleCommenter, false},
	{"editor", auth.KindUser, auth.RoleEditor, false},
	{"admin", auth.KindUser, auth.RoleAdmin, true},
	{"editor service account key", auth.KindServiceAccount, auth.RoleEditor, false},
	{"admin service account key", auth.KindServiceAccount, auth.RoleAdmin, true},
}

func adminsOnlyHarness(t *testing.T) (*harness, *fakeDatasource) {
	t.Helper()
	hn := newHarness(t)
	audit := &fakeDatasource{valuesFor: func(datasource.Query) []string { return []string{"secret"} }}
	if err := hn.h.registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: audit}); err != nil {
		t.Fatal(err)
	}
	restricted := testDashboard()
	restricted.Variables = append(restricted.Variables, spec.Variable{Name: "aj", Type: spec.QueryVariable, Datasource: "audit", Query: "label_values(up, job)"})
	restricted.Panels = append(restricted.Panels, spec.Panel{ID: "p_audit", Type: spec.Table, Title: "audit", Datasource: "audit", Queries: []spec.Query{{Query: "count(up)"}}})
	hn.specs.known[specKey{id: hn.id, version: auditVersion}] = restricted
	hn.specs.known[specKey{id: hn.id, version: pickedVersion}] = &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "picked",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Variables: []spec.Variable{
			{Name: "ds", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus},
			{Name: "dj", Type: spec.QueryVariable, Datasource: "${ds}", Query: "label_values(up, job)"},
		},
		Panels: []spec.Panel{{ID: "p_ds", Type: spec.Table, Title: "picked", Datasource: "${ds}", Queries: []spec.Query{{Query: "count(up)"}}}},
	}
	return hn, audit
}

func (hn *harness) as(r reader, req *http.Request) *httptest.ResponseRecorder {
	req.Header.Set("X-Kind", string(r.kind))
	req.Header.Set("X-Role", string(r.role))
	return hn.serve(req, "alice")
}

func (hn *harness) getAs(r reader, path string) *httptest.ResponseRecorder {
	return hn.as(r, httptest.NewRequest(http.MethodGet, path, nil))
}

func (hn *harness) postAs(r reader, path string, body any) *httptest.ResponseRecorder {
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	return hn.as(r, req)
}

func TestAnAdminsOnlyDatasourceAnswersANonAdminLikeAMissingOne(t *testing.T) {
	surfaces := map[string]func(hn *harness, r reader, uid string) *httptest.ResponseRecorder{
		"query": func(hn *harness, r reader, uid string) *httptest.ResponseRecorder {
			return hn.postAs(r, "/api/v1/datasources/"+uid+"/query", instantBody("up"))
		},
		"discovery": func(hn *harness, r reader, uid string) *httptest.ResponseRecorder {
			return hn.getAs(r, "/api/v1/datasources/"+uid+"/metrics")
		},
		"label values": func(hn *harness, r reader, uid string) *httptest.ResponseRecorder {
			return hn.getAs(r, "/api/v1/datasources/"+uid+"/labels/job/values")
		},
		"lint": func(hn *harness, r reader, uid string) *httptest.ResponseRecorder {
			return hn.postAs(r, "/api/v1/datasources/"+uid+"/lint", map[string]string{"query": "up"})
		},
	}
	for name, surface := range surfaces {
		for _, r := range readers {
			t.Run(name+"/"+r.name, func(t *testing.T) {
				hn, audit := adminsOnlyHarness(t)
				got := surface(hn, r, "audit")
				if r.reads {
					if got.Code == http.StatusNotFound {
						t.Fatalf("an Admin got %d: %s", got.Code, got.Body)
					}
					return
				}
				missing := surface(hn, r, "nope")
				if got.Code != missing.Code || got.Body.String() != missing.Body.String() {
					t.Fatalf("restricted answered %d %s, missing answered %d %s", got.Code, got.Body, missing.Code, missing.Body)
				}
				if ranges, instants, variables, lints := audit.counts(); ranges+instants+variables+lints != 0 || len(audit.discoverRequests()) != 0 {
					t.Fatal("a non-Admin request reached the admins only datasource")
				}
			})
		}
	}
}

func TestAVersionThatReadsAnAdminsOnlyDatasourceIsAdminOnly(t *testing.T) {
	for _, r := range readers {
		t.Run(r.name, func(t *testing.T) {
			hn, audit := adminsOnlyHarness(t)
			paths := []string{
				hn.versionPath(auditVersion, "/panels/p_ts/data?"+window+"&var-job=api"),
				hn.versionPath(auditVersion, "/panels/p_audit/data?"+window+"&var-job=api"),
				hn.versionPath(auditVersion, "/variables/job/values?"+window),
				hn.versionPath(auditVersion, "/variables/aj/values?"+window),
			}
			for _, path := range paths {
				rec := hn.getAs(r, path)
				if r.reads != (rec.Code == http.StatusOK) {
					t.Fatalf("%s: status %d %s", path, rec.Code, rec.Body)
				}
				if !r.reads && rec.Code != http.StatusNotFound {
					t.Fatalf("%s: status %d, want 404", path, rec.Code)
				}
			}
			if ranges, instants, variables, _ := audit.counts(); !r.reads && ranges+instants+variables != 0 {
				t.Fatal("a non-Admin request reached the admins only datasource")
			}
		})
	}
}

func TestAViewerNeverReceivesWhatAnAdminWarmed(t *testing.T) {
	admin := readers[3]
	viewer := readers[0]
	hn, audit := adminsOnlyHarness(t)
	paths := []string{
		hn.versionPath(pickedVersion, "/variables/dj/values?"+window),
		hn.versionPath(pickedVersion, "/variables/dj/values?"+window+"&var-ds=audit"),
		hn.versionPath(pickedVersion, "/panels/p_ds/data?"+window),
		hn.versionPath(pickedVersion, "/panels/p_ds/data?"+window+"&var-ds=audit"),
		hn.versionPath(auditVersion, "/variables/aj/values?"+window),
		hn.versionPath(auditVersion, "/panels/p_audit/data?"+window+"&var-job=api"),
	}
	for _, path := range paths {
		if rec := hn.getAs(admin, path); rec.Code != http.StatusOK {
			t.Fatalf("admin %s: status %d %s", path, rec.Code, rec.Body)
		}
	}
	ranges, instants, variables, _ := audit.counts()
	if ranges+instants+variables == 0 {
		t.Fatal("the Admin never read the admins only datasource")
	}
	for _, path := range paths {
		rec := hn.getAs(viewer, path)
		if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), `"cached":true`) {
			t.Fatalf("viewer %s: got %d %s", path, rec.Code, rec.Body)
		}
	}
	if r, i, v, _ := audit.counts(); r != ranges || i != instants || v != variables {
		t.Fatal("a viewer request reached the admins only datasource")
	}
}

func TestAPublicLinkToAVersionThatReadsAnAdminsOnlyDatasourceIsNotFound(t *testing.T) {
	hn, audit := adminsOnlyHarness(t)
	router := hn.publicRouter(fakePublicLinks{"audit": hn.publicLink(public.ModeFrozen, auditVersion, nil)})
	for _, path := range []string{
		"/api/v1/public/audit/panels/p_ts/data?" + window,
		"/api/v1/public/audit/panels/p_audit/data?" + window,
		"/api/v1/public/audit/variables/aj/values?" + window,
	} {
		expectError(t, publicGet(router, path), http.StatusNotFound, "not_found")
	}
	if ranges, instants, variables, _ := audit.counts(); ranges+instants+variables != 0 {
		t.Fatal("a public link reached the admins only datasource")
	}
}

const (
	flipVersion       = 4
	flipPickedVersion = 5
)

func flippingHarness(t *testing.T) (*harness, func(datasource.Audience)) {
	t.Helper()
	hn := newHarness(t)
	flip := datasource.Instance{UID: "flip", Datasource: &fakeDatasource{valuesFor: func(datasource.Query) []string { return []string{"secret"} }}}
	set := func(audience datasource.Audience) {
		t.Helper()
		flip.Audience = audience
		if err := hn.h.registry.Put(flip); err != nil {
			t.Fatal(err)
		}
	}
	set(datasource.AudienceEveryone)
	d := testDashboard()
	d.Variables = append(d.Variables, spec.Variable{Name: "fj", Type: spec.QueryVariable, Datasource: "flip", Query: "label_values(up, job)"})
	d.Panels = append(d.Panels, spec.Panel{ID: "p_flip", Type: spec.Table, Title: "flip", Datasource: "flip", Queries: []spec.Query{{Query: "count(up)"}}})
	hn.specs.known[specKey{id: hn.id, version: flipVersion}] = d
	hn.specs.known[specKey{id: hn.id, version: flipPickedVersion}] = &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "flip picked",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Variables: []spec.Variable{
			{Name: "fds", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus, Regex: "/^flip$/"},
			{Name: "fdj", Type: spec.QueryVariable, Datasource: "${fds}", Query: "label_values(up, job)"},
		},
	}
	return hn, set
}

func TestADatasourceSwitchedToAdminsIsClosedToEveryoneElseAtOnceThoughItsVersionWasCached(t *testing.T) {
	admin, viewer := readers[3], readers[0]
	hn, set := flippingHarness(t)
	values := hn.versionPath(flipVersion, "/variables/fj/values?"+window)
	paths := []string{values, hn.versionPath(flipVersion, "/panels/p_flip/data?"+window+"&var-job=api"), hn.versionPath(flipVersion, "/panels/p_ts/data?"+window+"&var-job=api")}
	for _, path := range paths {
		if rec := hn.getAs(viewer, path); rec.Code != http.StatusOK {
			t.Fatalf("viewer before the switch %s: status %d %s", path, rec.Code, rec.Body)
		}
	}
	set(datasource.AudienceAdmins)
	if rec := hn.getAs(admin, values); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("admin after the switch: status %d %s", rec.Code, rec.Body)
	}
	for _, path := range paths {
		if rec := hn.getAs(viewer, path); rec.Code != http.StatusNotFound {
			t.Fatalf("viewer after the switch %s: status %d %s", path, rec.Code, rec.Body)
		}
	}
	router := hn.publicRouter(fakePublicLinks{"flip": hn.publicLink(public.ModeFrozen, flipVersion, nil)})
	for _, path := range []string{"/api/v1/public/flip/variables/fj/values?" + window, "/api/v1/public/flip/panels/p_flip/data?" + window, "/api/v1/public/flip/panels/p_ts/data?" + window} {
		expectError(t, publicGet(router, path), http.StatusNotFound, "not_found")
	}
}

func TestValuesWarmedThroughADefaultPickedDatasourceAreNotServedOnceItBecomesAdminsOnly(t *testing.T) {
	viewer := readers[0]
	hn, set := flippingHarness(t)
	values := hn.versionPath(flipPickedVersion, "/variables/fdj/values?"+window)
	if rec := hn.getAs(viewer, values); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("viewer before the switch: status %d %s", rec.Code, rec.Body)
	}
	set(datasource.AudienceAdmins)
	if rec := hn.getAs(viewer, values); strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("viewer after the switch: status %d %s", rec.Code, rec.Body)
	}
}

func TestAnAdminsValuesAreNeverServedFromTheCacheToANonAdminWhoMayReadTheVariable(t *testing.T) {
	admin, viewer := readers[3], readers[0]
	hn, set := flippingHarness(t)
	set(datasource.AudienceAdmins)
	values := hn.versionPath(flipVersion, "/variables/fj/values?"+window)
	if rec := hn.getAs(admin, values); rec.Code != http.StatusOK {
		t.Fatalf("admin: status %d %s", rec.Code, rec.Body)
	}
	set(datasource.AudienceEveryone)
	if rec := hn.getAs(viewer, values); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"cached":true`) {
		t.Fatalf("viewer got what the Admin cached: status %d %s", rec.Code, rec.Body)
	}
}
