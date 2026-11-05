package imports_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/imports"
)

func withUID(raw []byte, uid string) []byte {
	return []byte(strings.Replace(string(raw), `"uid":"abc"`, `"uid":"`+uid+`"`, 1))
}

func TestOnlyTheFirstImportOfAGrafanaUIDHoldsIt(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop again", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.GrafanaUID != "abc" || second.Record.GrafanaUID != "" {
		t.Fatalf("grafana uids %q and %q, want abc and empty", first.Record.GrafanaUID, second.Record.GrafanaUID)
	}
	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, first.Dashboard.ID))
	if err != nil || stored.GrafanaUID != "abc" {
		t.Fatalf("stored grafana uid %q, err %v", stored.GrafanaUID, err)
	}
	found, ok, err := r.imports.DashboardByGrafanaUID(t.Context(), "abc")
	if err != nil || !ok || found.DashboardID.String() != first.Dashboard.ID || found.GrafanaVersion != 0 {
		t.Fatalf("by uid: %+v, found %t, err %v", found, ok, err)
	}

	if err := r.dashboard.Delete(t.Context(), ada, uuid(t, first.Dashboard.ID)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := r.imports.DashboardByGrafanaUID(t.Context(), "abc"); err != nil || ok {
		t.Fatalf("after deleting the holder: found %t, err %v", ok, err)
	}
	third, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop third", "prom", 1)})
	if err != nil || third.Record.GrafanaUID != "abc" {
		t.Fatalf("an import after the holder was deleted: uid %q, err %v", third.Record.GrafanaUID, err)
	}
}

func TestAGrafanaUIDOutsideGrafanasRulesIsNotHeld(t *testing.T) {
	for _, uid := range []string{"", "has space", "slash/uid", strings.Repeat("a", 41)} {
		t.Run(uid, func(t *testing.T) {
			r := newRig(t)
			got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: withUID(grafanaJSON("Shop", "prom", 1), uid)})
			if err != nil || got.Record.GrafanaUID != "" {
				t.Fatalf("grafana uid %q, err %v", got.Record.GrafanaUID, err)
			}
			if _, ok, err := r.imports.DashboardByGrafanaUID(t.Context(), uid); err != nil || ok {
				t.Fatalf("by uid: found %t, err %v", ok, err)
			}
		})
	}
}

func TestABatchImportNamesItsFolderAndGrafanaVersion(t *testing.T) {
	r := newRig(t)
	team, err := r.access.ForImport(t.Context(), ada, "Team")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.imports.Import(t.Context(), ada, imports.Request{
		JSON:           withUID(grafanaJSON("Shop", "prom", 1), "shop-1"),
		Folder:         "Ignored",
		FolderID:       team.Folder.ID,
		GrafanaVersion: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Dashboard.Folder.ID != team.Folder.ID {
		t.Errorf("folder %s, want Team %s", got.Dashboard.Folder.ID.String(), team.Folder.ID.String())
	}
	found, ok, err := r.imports.DashboardByGrafanaUID(t.Context(), "shop-1")
	if err != nil || !ok || found.GrafanaVersion != 7 {
		t.Fatalf("by uid: %+v, found %t, err %v", found, ok, err)
	}
}

func TestReimportSavesANewVersionAndRefreshesTheRecord(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: withUID(grafanaJSON("Shop", "prom", 1), "shop-1"), GrafanaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid(t, first.Dashboard.ID)
	got, err := r.imports.Reimport(t.Context(), ada, id, imports.Request{JSON: withUID(grafanaJSON("Shop v2", "prom", 2), "shop-1"), GrafanaVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version.Number != 2 || got.Version.Summary != `Updated "Shop v2" from Grafana` || got.Dashboard.Title != "Shop v2" {
		t.Fatalf("version %d, summary %q, title %q", got.Version.Number, got.Version.Summary, got.Dashboard.Title)
	}
	if got.Record.ID != first.Record.ID || got.Record.Version != 2 || got.Record.Title != "Shop v2" || got.Record.GrafanaUID != "shop-1" || got.Record.Report.Counts.Total != 2 || !got.Record.Report.Verified {
		t.Fatalf("record %+v", got.Record)
	}
	found, ok, err := r.imports.DashboardByGrafanaUID(t.Context(), "shop-1")
	if err != nil || !ok || found.GrafanaVersion != 2 || found.DashboardID != id {
		t.Fatalf("by uid: %+v, found %t, err %v", found, ok, err)
	}
}

func TestReimportRunsEveryQueryLikeAnyLaterSave(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	broken := []byte(strings.Replace(string(grafanaJSON("Shop", "prom", 1)), `"expr":"up"`, `"expr":"`+brokenQuery+`"`, 1))
	_, err = r.imports.Reimport(t.Context(), ada, uuid(t, first.Dashboard.ID), imports.Request{JSON: broken, GrafanaVersion: 2})
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidSpecError", err)
	}
	if latest, err := r.dashboard.LatestVersion(t.Context(), uuid(t, first.Dashboard.ID)); err != nil || latest != 1 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}
}
