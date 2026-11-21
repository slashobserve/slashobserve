package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func TestAFileDatasourceMadeAdminsOnlyStopsTheServerWhileSharedContentReadsIt(t *testing.T) {
	d := dbtest.New(t)
	var dashboard string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Salaries', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&dashboard); err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name) VALUES ($1::uuid, 1, '{"title":"Salaries","panels":[{"id":"p","type":"stat","datasource":"audit","queries":[{"query":"up"}]}]}', 'seed', 'user', gen_random_uuid(), 'seed')`, dashboard)
	everyone := []config.Datasource{{UID: "audit", Type: "prometheus"}}
	admins := []config.Datasource{{UID: "prom", Type: "prometheus"}, {UID: "audit", Type: "prometheus", Audience: datasource.AudienceAdmins}}

	if err := checkFileAudiences(t.Context(), d.Queries(), everyone); err != nil {
		t.Fatalf("everyone = %v, want a start", err)
	}

	record, _ := logExit(t, checkFileAudiences(t.Context(), d.Queries(), admins))
	detail, _ := record["detail"].(string)
	if record["error_code"] != "datasource_audience_refused" || record["datasource_uid"] != "audit" || record["count"] != float64(1) {
		t.Fatalf("record = %v", record)
	}
	for _, want := range []string{
		`makes "audit" Admins only, but 1 dashboard, proposal, observation or saved search outside folders only Admins can read uses it`,
		`dashboard ` + dashboard + ` "Salaries" in folder "General"`,
		"while the datasource is still everyone",
		"keep audience: everyone in the file",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail lacks %q:\n%s", want, detail)
		}
	}

	d.Exec(t, `UPDATE dashboards SET folder_id = $1 WHERE id = $2::uuid`, foldertest.AdminsOnly(t, d, "Payroll"), dashboard)
	if err := checkFileAudiences(t.Context(), d.Queries(), admins); err != nil {
		t.Fatalf("after the move = %v, want a start", err)
	}
}

func TestAnAppDatasourceAlreadyAdminsOnlyIsReportedAtStartWithoutTheContentOrARefusal(t *testing.T) {
	d := dbtest.New(t)
	d.Exec(t, `INSERT INTO datasources (uid, type, name, config, audience) VALUES ('payroll', 'prometheus', 'Payroll', '{}', 'admins'), ('ledger', 'prometheus', 'Ledger', '{}', 'admins'), ('audit', 'prometheus', 'Audit', '{}', 'admins')`)
	for _, uid := range []string{"payroll", "payroll", "ledger", "audit"} {
		d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name) VALUES ('metrics', 'Secret pay search', $1, 'up', 'now-1h', 'now', general_folder_id(), 'user', gen_random_uuid(), 'Ada')`, uid)
	}
	d.Exec(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name) VALUES ('metrics', 'Locked', 'ledger', 'up', 'now-1h', 'now', $1, 'user', gen_random_uuid(), 'Ada')`, foldertest.AdminsOnly(t, d, "Payroll"))
	file := []config.Datasource{{UID: "audit", Type: "prometheus", Audience: datasource.AudienceAdmins}}
	logger, logs := logtest.New()

	warnExposedAppDatasources(t.Context(), d.Queries(), file, logger)

	var got []string
	for _, r := range logs.Records(t) {
		if r["level"] != "WARN" || r["msg"] != "admins only datasource read outside admins only folders" {
			t.Fatalf("record = %v", r)
		}
		got = append(got, fmt.Sprintf("%v=%v/%v", r["datasource_uid"], r["shared_items"], r["personal_items"]))
	}
	if want := []string{"ledger=1/0", "payroll=2/0"}; !slices.Equal(got, want) {
		t.Fatalf("warnings = %v, want %v", got, want)
	}
	logs.AssertAbsent(t, "Secret pay search")
}
