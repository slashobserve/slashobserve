package grafanaconn_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

var corpusFiles = []string{
	"clickhouse-datasource/cluster-analysis.json",
	"clickhouse-datasource/otel-logs-explorer.json",
	"clickhouse-datasource/otel-service-dashboard.json",
	"clickhouse-datasource/otel-traces-explorer.json",
	"kube-prometheus/alertmanager-overview.json",
	"kube-prometheus/apiserver.json",
	"kube-prometheus/cluster-total.json",
	"kube-prometheus/controller-manager.json",
	"kube-prometheus/grafana-overview.json",
	"kube-prometheus/k8s-resources-cluster.json",
	"kube-prometheus/k8s-resources-multicluster.json",
	"kube-prometheus/k8s-resources-namespace.json",
	"kube-prometheus/k8s-resources-node.json",
	"kube-prometheus/k8s-resources-nodes-overview.json",
	"kube-prometheus/k8s-resources-pod.json",
	"kube-prometheus/k8s-resources-workload.json",
	"kube-prometheus/k8s-resources-workloads-namespace.json",
	"kube-prometheus/kubelet.json",
	"kube-prometheus/namespace-by-pod.json",
	"kube-prometheus/namespace-by-workload.json",
	"kube-prometheus/node-cluster-rsrc-use.json",
	"kube-prometheus/node-rsrc-use.json",
	"kube-prometheus/nodes.json",
	"kube-prometheus/persistentvolumesusage.json",
	"kube-prometheus/pod-total.json",
	"kube-prometheus/prometheus-remote-write.json",
	"kube-prometheus/prometheus.json",
	"kube-prometheus/proxy.json",
	"kube-prometheus/scheduler.json",
	"kube-prometheus/workload-total.json",
}

var folderTitles = map[string]string{"f-apps": "Apps", "f-plat": "Platform", "f-k8s": "Platform / Kubernetes", "": "General"}

func corpusModel(t *testing.T, name string) (map[string]any, string) {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS(filepath.Join("..", "..", "pkg", "grafana", "testdata", "corpus")), name)
	if err != nil {
		t.Fatal(err)
	}
	var model map[string]any
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	title, _ := model["title"].(string)
	delete(model, "id")
	return model, title
}

func serveCorpus(t *testing.T, g *fakeGrafana) []string {
	t.Helper()
	threeFolders(g)
	folderOrder := []string{"f-apps", "f-plat", "f-k8s"}
	uids := make([]string, 0, len(corpusFiles))
	for i, name := range corpusFiles {
		model, title := corpusModel(t, name)
		uid := numbered("corpus", i)
		g.add(&fakeDashboard{uid: uid, title: title, folder: folderOrder[i%len(folderOrder)], model: model, version: 3})
		uids = append(uids, uid)
	}
	return uids
}

func outcomes(s grafanaconn.Sync) map[string]grafanaconn.SyncResult {
	out := map[string]grafanaconn.SyncResult{}
	for _, res := range s.Results {
		out[res.UID] = res
	}
	return out
}

func TestSyncImportsEveryDashboardIntoItsFolder(t *testing.T) {
	r := newRig(t)
	uids := serveCorpus(t, r.grafana)
	root := simpleModel("graf-prom", "prometheus")
	root["tags"] = []any{"edge", "checkout"}
	root["links"] = []any{map[string]any{"title": "Runbook", "type": "link", "url": "https://runbooks.example.com/checkout"}}
	r.grafana.add(&fakeDashboard{uid: "root", title: "Edge", model: root, version: 2})
	r.grafana.add(&fakeDashboard{uid: "broken", title: "Broken", folder: "f-apps", model: simpleModel("graf-prom", "prometheus"), version: 1, status: 500})
	conn := r.connect(t)

	done := r.finish(t, conn, grafanaconn.SyncRequest{All: true})
	if done.Status != grafanaconn.StatusFinished || done.Total != 32 || done.Done != 32 || done.Failed != 1 || done.Skipped != 0 || done.FinishedAt == nil {
		t.Fatalf("summary %+v", done.SyncSummary)
	}
	byUID := outcomes(done)
	if res := byUID["broken"]; res.Outcome != grafanaconn.OutcomeFailed || res.Error != "Grafana answered with status 500." || res.DashboardID != "" {
		t.Fatalf("broken result %+v", res)
	}
	for i, uid := range append(slices.Clone(uids), "root") {
		res := byUID[uid]
		if res.Outcome != grafanaconn.OutcomeImported || res.DashboardID == "" || res.Error != "" {
			t.Fatalf("result for %s: %+v", uid, res)
		}
		want := folderTitles[""]
		if uid != "root" {
			want = folderTitles[[]string{"f-apps", "f-plat", "f-k8s"}[i%3]]
		}
		if got := r.folderOf(t, uid); got != want {
			t.Errorf("%s landed in %q, want %q", uid, got, want)
		}
	}
	if n := r.count(t, "SELECT count(*) FROM imports WHERE grafana_uid <> ''"); n != 31 {
		t.Fatalf("%d import records hold a grafana uid, want 31", n)
	}
	if n := r.count(t, "SELECT count(*) FROM imports"); n != 31 {
		t.Fatalf("%d import records, want 31", n)
	}
	if n := r.count(t, `SELECT count(*) FROM imports WHERE (summary->>'grafana_version')::int = 3`); n != 30 {
		t.Fatalf("%d records store grafana version 3, want 30", n)
	}
	if n := r.count(t, `
		SELECT count(*) FROM dashboards d JOIN imports i ON i.dashboard_id = d.id JOIN dashboard_versions v ON v.dashboard_id = d.id
		WHERE i.grafana_uid = 'root' AND d.tags = ARRAY['edge', 'checkout'] AND jsonb_array_length(v.spec->'links') = 1`); n != 1 {
		t.Fatal("the root dashboard lost its tags or links")
	}

	stored, err := r.svc.Get(t.Context(), uuidOf(t, conn.ID))
	if err != nil || stored.LastSync == nil || stored.LastSync.ID != done.ID || stored.LastSync.Status != grafanaconn.StatusFinished || stored.LastSync.Failed != 1 {
		t.Fatalf("last sync %+v, err %v", stored.LastSync, err)
	}
}

func TestSyncSkipsUnchangedDashboardsAndUpdatesNewerOnes(t *testing.T) {
	r := newRig(t)
	threeFolders(r.grafana)
	for i := range 3 {
		r.grafana.add(&fakeDashboard{uid: numbered("d", i), title: numbered("Dash", i), folder: "f-apps", model: simpleModel("graf-prom", "prometheus"), version: 1})
	}
	conn := r.connect(t)
	if first := r.finish(t, conn, grafanaconn.SyncRequest{All: true}); first.Failed != 0 || first.Done != 3 {
		t.Fatalf("first run %+v", first.SyncSummary)
	}

	again := r.finish(t, conn, grafanaconn.SyncRequest{All: true})
	if again.Status != grafanaconn.StatusFinished || again.Skipped != 3 || again.Failed != 0 {
		t.Fatalf("second run %+v", again.SyncSummary)
	}

	r.grafana.bump("d-1")
	third := r.finish(t, conn, grafanaconn.SyncRequest{All: true})
	results := outcomes(third)
	if third.Skipped != 2 || results["d-1"].Outcome != grafanaconn.OutcomeUpdated {
		t.Fatalf("third run %+v, results %+v", third.SyncSummary, third.Results)
	}
	if n := r.count(t, `SELECT d.latest_version FROM dashboards d JOIN imports i ON i.dashboard_id = d.id WHERE i.grafana_uid = 'd-1'`); n != 2 {
		t.Fatalf("latest version %d, want 2", n)
	}
	if n := r.count(t, "SELECT count(*) FROM imports"); n != 3 {
		t.Fatalf("%d import records, want 3", n)
	}
}

func TestSyncImportsOnlyTheChosenFolders(t *testing.T) {
	r := newRig(t)
	threeFolders(r.grafana)
	for i, folder := range []string{"f-apps", "f-plat", "f-k8s", ""} {
		r.grafana.add(&fakeDashboard{uid: numbered("d", i), title: numbered("Dash", i), folder: folder, model: simpleModel("graf-prom", "prometheus"), version: 1})
	}
	conn := r.connect(t)

	done := r.finish(t, conn, grafanaconn.SyncRequest{Folders: []string{"f-plat", "general"}})
	var got []string
	for _, res := range done.Results {
		got = append(got, res.UID)
	}
	slices.Sort(got)
	if want := []string{"d-1", "d-2", "d-3"}; !slices.Equal(got, want) || done.Total != 3 {
		t.Fatalf("imported %v (total %d), want %v", got, done.Total, want)
	}
}

func TestSyncRefusesAMappingToAnUnknownDatasource(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	_, err := r.svc.StartSync(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.SyncRequest{All: true, Mapping: map[string]string{"graf-prom": "nope"}})
	var invalid *grafanaconn.InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("error %v, want InvalidError", err)
	}
}

func TestSyncRunsOneAtATimePerConnection(t *testing.T) {
	hold := make(chan struct{})
	r := newRigWith(t, rigOptions{secretKey: secretKey, hold: hold})
	r.grafana.add(&fakeDashboard{uid: "d", title: "Dash", model: simpleModel("graf-prom", "prometheus"), version: 1})
	conn := r.connect(t)

	started, err := r.svc.StartSync(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.SyncRequest{All: true})
	if err != nil || started.Status != grafanaconn.StatusRunning {
		t.Fatalf("started %+v, err %v", started.SyncSummary, err)
	}
	if _, err := r.svc.StartSync(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.SyncRequest{All: true}); !errors.Is(err, grafanaconn.ErrSyncRunning) {
		t.Fatalf("second start: %v, want ErrSyncRunning", err)
	}
	close(hold)
	r.svc.Wait()
	done, err := r.svc.SyncStatus(uuidOf(t, conn.ID), started.ID)
	if err != nil || done.Status != grafanaconn.StatusFinished || done.Done != 1 {
		t.Fatalf("done %+v, err %v", done.SyncSummary, err)
	}
	if _, err := r.svc.SyncStatus(uuidOf(t, conn.ID), "unknown"); !errors.Is(err, grafanaconn.ErrSyncNotFound) {
		t.Fatalf("unknown sync: %v", err)
	}
}

func TestSyncStopsWhenTheServerShutsDown(t *testing.T) {
	hold := make(chan struct{})
	r := newRigWith(t, rigOptions{secretKey: secretKey, hold: hold})
	r.grafana.add(&fakeDashboard{uid: "d", title: "Dash", model: simpleModel("graf-prom", "prometheus"), version: 1})
	conn := r.connect(t)

	started, err := r.svc.StartSync(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.SyncRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	r.stop()
	r.svc.Wait()
	done, err := r.svc.SyncStatus(uuidOf(t, conn.ID), started.ID)
	if err != nil || done.Status != grafanaconn.StatusCanceled {
		t.Fatalf("done %+v, err %v", done.SyncSummary, err)
	}
	stored, err := r.svc.Get(t.Context(), uuidOf(t, conn.ID))
	if err != nil || stored.LastSync == nil || stored.LastSync.Status != grafanaconn.StatusCanceled {
		t.Fatalf("last sync %+v, err %v", stored.LastSync, err)
	}
	if n := r.count(t, "SELECT count(*) FROM imports"); n != 0 {
		t.Fatalf("%d imports after shutdown", n)
	}
}

func TestSyncFailsWhenGrafanaCannotBeListed(t *testing.T) {
	r := newRig(t)
	r.grafana.token = "glsa_rotated"
	conn := r.connect(t)
	done := r.finish(t, conn, grafanaconn.SyncRequest{All: true})
	if done.Status != grafanaconn.StatusFailed || done.Error != "Grafana refused the token (status 401)." || done.Total != 0 {
		t.Fatalf("summary %+v", done.SyncSummary)
	}
}
