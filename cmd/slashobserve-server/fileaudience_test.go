package main

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
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
		`makes "audit" Admins only, but 1 dashboards`,
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
