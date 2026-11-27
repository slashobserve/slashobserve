package datasourceadmin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

func TestCreateStoresOnlyCiphertextAndStartsUsingTheDatasource(t *testing.T) {
	var auth string
	server := upstream(t, &auth)
	r, svc := newRig(t, testKey)

	record, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{BearerToken: "token-1", Headers: map[string]string{"X-Org": "org-7"}})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"bearer_token", "header:X-Org"}; strings.Join(record.SecretsSet, ",") != strings.Join(want, ",") {
		t.Fatalf("secrets set = %v, want %v", record.SecretsSet, want)
	}
	row, err := r.db.Queries().GetDatasource(reading(t.Context()), "added")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(row.Secrets), "token-1") || strings.Contains(string(row.Config), "token-1") || strings.Contains(string(row.Config), "org-7") {
		t.Fatal("the table holds a plain credential")
	}
	if status := health(t, r, "added"); !status.Healthy || status.Source != datasource.SourceApp {
		t.Fatalf("status = %+v", status)
	}
	if auth != "Bearer token-1" {
		t.Fatalf("the upstream saw Authorization %q", auth)
	}
}

func TestCreateWithoutAKeyRefusesCredentialsButSavesAnOpenDatasource(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, nil)

	_, err := svc.Create(reading(t.Context()), r.admin, prometheus("secured", server.URL), Secrets{BearerToken: "token-1"})
	if !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("error = %v, want ErrNoSecretKey", err)
	}
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("open", server.URL), Secrets{}); err != nil {
		t.Fatalf("a datasource without credentials needs no key: %v", err)
	}
}

func TestCreateRefusesATakenUIDAndNeverDuplicates(t *testing.T) {
	server := upstream(t, nil)
	file := datasource.Instance{UID: "fromfile", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{}); err != nil {
		t.Fatal(err)
	}

	for _, uid := range []string{"added", "fromfile"} {
		if _, err := svc.Create(reading(t.Context()), r.admin, prometheus(uid, server.URL), Secrets{}); !errors.Is(err, ErrUIDTaken) {
			t.Fatalf("Create(%q) = %v, want ErrUIDTaken", uid, err)
		}
	}
	if n, _ := r.db.Queries().CountDatasources(reading(t.Context())); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestCreateNamesTheFieldsThatAreWrong(t *testing.T) {
	r, svc := newRig(t, testKey)
	tests := []struct {
		name  string
		def   Definition
		field string
	}{
		{"no address", Definition{UID: "p", Type: TypePrometheus}, "address"},
		{"bad scheme", prometheus("p", "ftp://prom:9090"), "address"},
		{"login in the address", prometheus("p", "http://bob:pw@prom:9090"), "address"},
		{"link-local", prometheus("p", "http://169.254.169.254/latest"), "address"},
		{"unspecified", prometheus("p", "http://0.0.0.0:9090"), "address"},
		{"multicast", prometheus("p", "http://224.0.0.1:9090"), "address"},
		{"bad uid", prometheus("has space", "http://prom:9090"), "uid"},
		{"unknown type", Definition{UID: "p", Type: "graphite", Address: "http://g:80"}, "type"},
		{"clickhouse address", Definition{UID: "c", Type: TypeClickHouse, Address: "no-port", Username: "u"}, "address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(reading(t.Context()), r.admin, tt.def, Secrets{Password: "pw"})
			var invalidErr *InvalidError
			if !errors.As(err, &invalidErr) {
				t.Fatalf("error = %v, want InvalidError", err)
			}
			found := false
			for _, f := range invalidErr.Fields {
				found = found || f.Field == tt.field
			}
			if !found {
				t.Fatalf("fields = %v, want one named %q", invalidErr.Fields, tt.field)
			}
		})
	}
}

func TestOutboundAllowlistDecidesWhichDomainsCanBeAdded(t *testing.T) {
	listed, _ := domains.ParsePattern("prom.example.com")
	r, svc := newRigWith(t, rigOptions{key: testKey, lock: allowlist.Lock{Locked: true, Domains: domains.List{listed}}})

	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("ok", "http://prom.example.com:9090"), Secrets{}); err != nil {
		t.Fatalf("a listed domain: %v", err)
	}
	for _, address := range []string{"http://other.example.com:9090", "http://10.1.2.3:9090", "http://prometheus:9090"} {
		if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("no", address), Secrets{}); err == nil || errors.Is(err, ErrUIDTaken) {
			t.Fatalf("Create(%q) = %v, want a refusal by the locked allowlist", address, err)
		}
	}
	if n, _ := r.db.Queries().CountDatasources(reading(t.Context())); n != 1 {
		t.Fatalf("rows = %d, want only the listed one", n)
	}
}

func TestAnUnlockedAllowlistGainsTheDomainAndTakesIPsAndSingleLabelNames(t *testing.T) {
	r, svc := newRig(t, testKey)

	for uid, address := range map[string]string{"named": "https://prom.example.com", "ip": "http://10.1.2.3:9090", "service": "http://prometheus:9090"} {
		if _, err := svc.Create(reading(t.Context()), r.admin, prometheus(uid, address), Secrets{}); err != nil {
			t.Fatalf("Create(%q): %v", address, err)
		}
	}
	list, err := r.domains.Allowed(reading(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	if !list.Allows("prom.example.com") || len(list) != 1 {
		t.Fatalf("allowlist = %v, want only prom.example.com", list)
	}
}

func TestUpdateKeepsCredentialsItIsNotToldToChange(t *testing.T) {
	var auth string
	server := upstream(t, &auth)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{BearerToken: "token-1", Headers: map[string]string{"X-Org": "org-7"}}); err != nil {
		t.Fatal(err)
	}

	def := prometheus("added", server.URL)
	def.Name = "Renamed"
	record, err := svc.Update(reading(t.Context()), r.admin, "added", def, SecretsChange{})
	if err != nil {
		t.Fatal(err)
	}
	if record.Definition.Name != "Renamed" || len(record.SecretsSet) != 2 {
		t.Fatalf("record = %+v", record)
	}
	if in, _ := registered(t, r.registry, "added"); in.Name != "Renamed" {
		t.Fatalf("registered name = %q", in.Name)
	}
	health(t, r, "added")
	if auth != "Bearer token-1" {
		t.Fatalf("credential after a rename = %q", auth)
	}

	if _, err := svc.Update(reading(t.Context()), r.admin, "added", def, SecretsChange{Set: Secrets{BearerToken: "token-2"}, Clear: []string{"header:X-Org"}}); err != nil {
		t.Fatal(err)
	}
	record, _ = svc.Get(reading(t.Context()), "added")
	if strings.Join(record.SecretsSet, ",") != "bearer_token" {
		t.Fatalf("secrets set = %v, want only the bearer token", record.SecretsSet)
	}
	r.registry.Recheck(reading(t.Context()), "added")
	status, _ := r.registry.Recheck(reading(t.Context()), "added")
	if !status.Healthy || auth != "Bearer token-2" {
		t.Fatalf("status = %+v, Authorization = %q", status, auth)
	}
}

func TestUpdateAndDeleteRefuseFileDatasourcesAndUnknownOnes(t *testing.T) {
	file := datasource.Instance{UID: "fromfile", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable), Source: datasource.SourceFile}
	r, svc := newRig(t, testKey, file)

	if _, err := svc.Update(reading(t.Context()), r.admin, "fromfile", prometheus("fromfile", "http://prom:9090"), SecretsChange{}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Update of a file datasource = %v", err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "fromfile"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Delete of a file datasource = %v", err)
	}
	if _, err := svc.Get(reading(t.Context()), "fromfile"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Get of a file datasource = %v", err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "nothing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete of an unknown datasource = %v", err)
	}
}

func TestAStoredRowCannotShadowTheBuiltInDatasource(t *testing.T) {
	builtIn := datasource.Instance{UID: "audit", Datasource: datasource.NewUnavailable("opensearch", datasource.ErrUnavailable), Source: datasource.SourceBuiltin, Audience: datasource.AudienceAdmins}
	r, svc := newRig(t, testKey, builtIn)
	r.insert(t, testKey, prometheus("audit", "http://prom:9090"), Secrets{})

	if _, err := svc.Update(reading(t.Context()), r.admin, "audit", prometheus("audit", "http://prom:9091"), SecretsChange{}); !errors.Is(err, ErrBuiltIn) {
		t.Fatalf("Update = %v, want ErrBuiltIn", err)
	}
	if err := svc.Delete(reading(t.Context()), r.admin, "audit"); !errors.Is(err, ErrBuiltIn) {
		t.Fatalf("Delete = %v, want ErrBuiltIn", err)
	}
	if _, err := svc.Get(reading(t.Context()), "audit"); !errors.Is(err, ErrBuiltIn) {
		t.Fatalf("Get = %v, want ErrBuiltIn", err)
	}
	row, err := r.db.Queries().GetDatasource(reading(t.Context()), "audit")
	if err != nil || row.Name != "Metrics audit" {
		t.Fatalf("stored row = %+v, %v, want it untouched", row, err)
	}
}

func TestUpdateCannotChangeUIDOrType(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{}); err != nil {
		t.Fatal(err)
	}
	changed := prometheus("added", server.URL)
	changed.Type = TypeLoki
	if _, err := svc.Update(reading(t.Context()), r.admin, "added", changed, SecretsChange{}); !errors.Is(err, ErrIdentityChanged) {
		t.Fatalf("error = %v, want ErrIdentityChanged", err)
	}
}

func TestDeleteRefusesADatasourceThatADashboardUses(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{}); err != nil {
		t.Fatal(err)
	}
	addDashboard(t, r, "Checkout", "added")

	err := svc.Delete(reading(t.Context()), r.admin, "added")
	var inUse *InUseError
	if !errors.As(err, &inUse) || len(inUse.Users) != 1 || inUse.Users[0].Title != "Checkout" || inUse.Users[0].Kind != "dashboard" {
		t.Fatalf("error = %v, want the dashboard named in an InUseError", err)
	}
	if _, ok := registered(t, r.registry, "added"); !ok {
		t.Fatal("a refused removal dropped the datasource")
	}
}

func TestDeleteRemovesAnUnusedDatasourceAtOnce(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(reading(t.Context()), r.admin, "added"); err != nil {
		t.Fatal(err)
	}
	if _, ok := registered(t, r.registry, "added"); ok {
		t.Fatal("the removed datasource is still registered")
	}
	if n, _ := r.db.Queries().CountDatasources(reading(t.Context())); n != 0 {
		t.Fatalf("rows = %d", n)
	}
}

func TestTestReportsAWorkingAndABrokenDraftWithoutSavingEither(t *testing.T) {
	server := upstream(t, nil)
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	r, svc := newRig(t, testKey)

	good, err := svc.Test(reading(t.Context()), "", prometheus("draft", server.URL), SecretsChange{})
	if err != nil || !good.Healthy {
		t.Fatalf("working draft = %+v, %v", good, err)
	}
	bad, err := svc.Test(reading(t.Context()), "", prometheus("draft", downURL), SecretsChange{})
	if err != nil || bad.Healthy || bad.Message == "" || strings.Contains(bad.Message+bad.Detail, downURL) {
		t.Fatalf("broken draft = %+v, %v", bad, err)
	}
	if good.FailureKind != "" || bad.FailureKind != datasource.FailureTransient {
		t.Fatalf("failure kinds = %q for the working draft and %q for the unreachable one", good.FailureKind, bad.FailureKind)
	}
	if n, _ := r.db.Queries().CountDatasources(reading(t.Context())); n != 0 {
		t.Fatalf("a test saved %d rows", n)
	}
}

func TestTestFromASavedDatasourceUsesItsStoredCredentials(t *testing.T) {
	var auth string
	server := upstream(t, &auth)
	r, svc := newRig(t, testKey)
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{BearerToken: "token-1"}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.Test(reading(t.Context()), "added", prometheus("added", server.URL), SecretsChange{})
	if err != nil || !result.Healthy || auth != "Bearer token-1" {
		t.Fatalf("result = %+v, %v, Authorization = %q", result, err, auth)
	}
}

func TestAliasesCannotShadowAnotherDatasource(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	first := prometheus("first", server.URL)
	first.GrafanaUIDs = []string{"grafana-prom"}
	if _, err := svc.Create(reading(t.Context()), r.admin, first, Secrets{}); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"grafana-prom", "first"} {
		second := prometheus("second", server.URL)
		second.GrafanaUIDs = []string{alias}
		var invalidErr *InvalidError
		if _, err := svc.Create(reading(t.Context()), r.admin, second, Secrets{}); !errors.As(err, &invalidErr) {
			t.Fatalf("alias %q: error = %v, want InvalidError", alias, err)
		}
	}
}

func TestCredentialsThatTogetherOutgrowTheStoredColumnAreRefusedByName(t *testing.T) {
	r, svc := newRig(t, testKey)
	server := upstream(t, nil)
	headers := func(prefix string, n int) map[string]string {
		out := map[string]string{}
		for i := range n {
			out[fmt.Sprintf("%s-%d", prefix, i)] = strings.Repeat("v", 4000)
		}
		return out
	}
	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("added", server.URL), Secrets{Headers: headers("A", 10)}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Update(reading(t.Context()), r.admin, "added", prometheus("added", server.URL), SecretsChange{Set: Secrets{Headers: headers("B", 10)}})

	var invalidErr *InvalidError
	if !errors.As(err, &invalidErr) || invalidErr.Fields[0].Field != "secrets.headers" {
		t.Fatalf("error = %v, want invalid_datasource naming secrets.headers", err)
	}
}

func TestTestingAnAddressNeverAddsItsDomainToTheAllowlist(t *testing.T) {
	r, svc := newRig(t, testKey)

	result, err := svc.Test(reading(t.Context()), "", prometheus("draft", "http://typo-host.example.invalid:9090"), SecretsChange{})
	if err != nil || result.Healthy {
		t.Fatalf("result = %+v, error = %v", result, err)
	}

	list, err := r.domains.Allowed(reading(t.Context()))
	if err != nil || len(list) != 0 {
		t.Fatalf("allowlist after a test = %v, error = %v", list, err)
	}
}

func TestTestingARefusedDomainStillAnswersTheLockedAllowlist(t *testing.T) {
	listed, _ := domains.ParsePattern("prom.example.com")
	_, svc := newRigWith(t, rigOptions{key: testKey, lock: allowlist.Lock{Locked: true, Domains: domains.List{listed}}})

	if _, err := svc.Test(reading(t.Context()), "", prometheus("draft", "http://other.example.com:9090"), SecretsChange{}); err == nil {
		t.Fatal("a domain outside the locked list was tested")
	}
	if _, err := svc.Test(reading(t.Context()), "", prometheus("draft", "http://10.1.2.3:9090"), SecretsChange{}); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("an IP address on a locked list = %v, want ErrAddressRefused", err)
	}
}

func TestACreateThatFailsAfterAddingItsDomainTakesTheDomainBack(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	r.insert(t, testKey, prometheus("raced", server.URL), Secrets{})

	_, err := svc.Create(reading(t.Context()), r.admin, prometheus("raced", "http://prom.example.invalid:9090"), Secrets{})
	if !errors.Is(err, ErrUIDTaken) {
		t.Fatalf("error = %v, want ErrUIDTaken", err)
	}

	list, err := r.domains.Allowed(reading(t.Context()))
	if err != nil || len(list) != 0 {
		t.Fatalf("allowlist after a failed create = %v, error = %v", list, err)
	}
}

func TestACreateThatFailsKeepsADomainThatWasAlreadyAllowed(t *testing.T) {
	server := upstream(t, nil)
	r, svc := newRig(t, testKey)
	pattern, _ := domains.ParsePattern("prom.example.invalid")
	if _, err := r.domains.Add(reading(t.Context()), r.admin, pattern); err != nil {
		t.Fatal(err)
	}
	r.insert(t, testKey, prometheus("raced", server.URL), Secrets{})

	if _, err := svc.Create(reading(t.Context()), r.admin, prometheus("raced", "http://prom.example.invalid:9090"), Secrets{}); !errors.Is(err, ErrUIDTaken) {
		t.Fatalf("error = %v, want ErrUIDTaken", err)
	}

	if list, _ := r.domains.Allowed(reading(t.Context())); len(list) != 1 {
		t.Fatalf("allowlist = %v, want the entry that was there before", list)
	}
}
