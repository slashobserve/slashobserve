package grafanaconn_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

func threeFolders(g *fakeGrafana) {
	g.folders = []fakeFolder{
		{uid: "f-plat", title: "Platform"},
		{uid: "f-k8s", title: "Kubernetes", parent: "f-plat"},
		{uid: "f-apps", title: "Apps"},
	}
}

func TestFoldersNestTitlesAndCountDashboards(t *testing.T) {
	r := newRig(t)
	threeFolders(r.grafana)
	for i, folder := range []string{"f-apps", "f-apps", "f-k8s", ""} {
		r.grafana.add(&fakeDashboard{uid: numbered("d", i), title: numbered("Dash", i), folder: folder, model: simpleModel("graf-prom", "prometheus"), version: 1})
	}
	conn := r.connect(t)

	got, err := r.svc.Folders(t.Context(), uuidOf(t, conn.ID))
	if err != nil {
		t.Fatal(err)
	}
	want := []grafanaconn.Folder{
		{UID: "f-apps", Title: "Apps", Dashboards: 2},
		{UID: "general", Title: "General", Dashboards: 1},
		{UID: "f-plat", Title: "Platform", Dashboards: 0},
		{UID: "f-k8s", Title: "Platform / Kubernetes", Dashboards: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("folders\n got %+v\nwant %+v", got, want)
	}
}

func TestFoldersOfAGrafanaThatRefusesTheToken(t *testing.T) {
	r := newRig(t)
	r.grafana.token = "glsa_rotated"
	conn := r.connect(t)
	_, err := r.svc.Folders(t.Context(), uuidOf(t, conn.ID))
	var status *egress.StatusError
	if !errors.As(err, &status) || status.Status != 401 {
		t.Fatalf("error %v, want a 401 status error", err)
	}
}

func TestDatasourcesShowTheAutomaticMatch(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	got, err := r.svc.Datasources(t.Context(), uuidOf(t, conn.ID))
	if err != nil {
		t.Fatal(err)
	}
	want := []grafanaconn.DatasourceMatch{
		{UID: "graf-ch", Name: "ClickHouse", Type: "grafana-clickhouse-datasource", Kind: "clickhouse", Candidates: []string{"ch"}, MatchedUID: "ch"},
		{UID: "graf-loki", Name: "Loki", Type: "loki", Candidates: []string{}},
		{UID: "graf-prom", Name: "Prometheus", Type: "prometheus", Kind: "prometheus", Candidates: []string{"prom"}, MatchedUID: "prom"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("datasources\n got %+v\nwant %+v", got, want)
	}
}

func TestDatasourcesOfAnUnknownConnection(t *testing.T) {
	r := newRig(t)
	if _, err := r.svc.Datasources(t.Context(), uuidOf(t, "6d2c1b9e-6a55-4f3e-9f2b-6a8d2f1e0c11")); !errors.Is(err, grafanaconn.ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
}
