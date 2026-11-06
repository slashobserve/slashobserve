package dashboards

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func rebaseDoc(t *testing.T, title string, panels ...string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse(fmt.Appendf(nil, `{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, strings.Join(panels, ",")))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func timeseries(id, title, query string, x, y, w int) string {
	return fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":8},"datasource":"prom","queries":[{"query":%q}]}`, id, title, x, y, w, query)
}

func row(id string, y int, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, id, y, strings.Join(children, ","))
}

func TestRebaseSpec(t *testing.T) {
	p1 := timeseries("p1", "p1", "up", 0, 0, 12)
	p2 := timeseries("p2", "p2", "up", 12, 0, 12)
	p3 := timeseries("p3", "p3", "up", 0, 9, 12)
	base := func(t *testing.T) *spec.Dashboard { return rebaseDoc(t, "t", p1, p2, row("r1", 8, p3)) }

	tests := []struct {
		name   string
		build  []string
		latest []string
		title  string
		want   []string
	}{
		{
			name:   "disjoint edits keep both",
			build:  []string{timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)},
			latest: []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
			want:   []string{timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
		},
		{
			name:   "a panel the build added to a row lands in that row at its place",
			build:  []string{p1, p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))},
			latest: []string{timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3)},
			want:   []string{timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))},
		},
		{
			name:   "a panel the build deleted stays deleted",
			build:  []string{p1, row("r1", 8, p3)},
			latest: []string{p1, p2, row("r1", 8, timeseries("p3", "renamed", "up", 0, 9, 12))},
			want:   []string{p1, row("r1", 8, timeseries("p3", "renamed", "up", 0, 9, 12))},
		},
		{
			name:   "a build resize is carried",
			build:  []string{timeseries("p1", "p1", "up", 0, 0, 6), p2, row("r1", 8, p3)},
			latest: []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
			want:   []string{timeseries("p1", "p1", "up", 0, 0, 6), timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
		},
		{
			name:   "a newer move of a panel the build edited keeps the newer place",
			build:  []string{timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)},
			latest: []string{timeseries("p1", "p1", "up", 0, 20, 12), p2, row("r1", 8, p3)},
			want:   []string{timeseries("p1", "p1", "rate(x[5m])", 0, 20, 12), p2, row("r1", 8, p3)},
		},
		{
			name:   "an unchanged build takes the newer version",
			build:  []string{p1, p2, row("r1", 8, p3)},
			latest: []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
			want:   []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
		},
		{
			name:   "both changing one panel is no rebase",
			build:  []string{timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)},
			latest: []string{timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3)},
		},
		{
			name:   "a build that deletes what the newer version edited is no rebase",
			build:  []string{p1, row("r1", 8, p3)},
			latest: []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
		},
		{
			name:   "both adding the same id is no rebase",
			build:  []string{p1, p2, row("r1", 8, p3), timeseries("p9", "mine", "up", 0, 30, 12)},
			latest: []string{p1, p2, row("r1", 8, p3), timeseries("p9", "theirs", "up", 0, 30, 12)},
		},
		{
			name:   "a build that changed a dashboard field is no rebase",
			title:  "retitled",
			build:  []string{timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)},
			latest: []string{p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)},
		},
		{
			name:   "a build panel that overlaps a newer panel is no rebase",
			build:  []string{p1, p2, row("r1", 8, p3), timeseries("p4", "p4", "up", 0, 30, 12)},
			latest: []string{p1, p2, row("r1", 8, p3), timeseries("p5", "p5", "up", 0, 30, 12)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := tt.title
			if title == "" {
				title = "t"
			}
			merged, ok, err := rebaseSpec(base(t), rebaseDoc(t, title, tt.build...), rebaseDoc(t, "t", tt.latest...))
			if err != nil {
				t.Fatal(err)
			}
			if ok != (tt.want != nil) {
				t.Fatalf("rebased = %v, want %v", ok, tt.want != nil)
			}
			if !ok {
				return
			}
			if got, want := canonicalJSON(t, merged), canonicalJSON(t, rebaseDoc(t, "t", tt.want...)); got != want {
				t.Fatalf("merged\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func canonicalJSON(t *testing.T, d *spec.Dashboard) string {
	t.Helper()
	states, err := panelStates(d)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(states)
}

func (r *rig) saveConcurrent(t *testing.T, dash pgtype.UUID, parent int, raw []byte) {
	t.Helper()
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, parent, raw, "concurrent edit"); err != nil {
		t.Fatal(err)
	}
}

func renamed(raw []byte, id, title string) []byte {
	return bytes.Replace(raw, fmt.Appendf(nil, `"title":%q`, id), fmt.Appendf(nil, `"title":%q`, title), 1)
}

func requeried(raw []byte, id string) []byte {
	return bytes.Replace(raw, fmt.Appendf(nil, `"title":%q,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]`, id),
		fmt.Appendf(nil, `"title":%q,"gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"rate(errors[5m])"}]`, id), 1)
}

func TestARebaseMovesTheBuildOntoADisjointNewerVersion(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))

	onto, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || !rebased || onto != 2 {
		t.Fatalf("rebase = onto %d, rebased %v, err %v", onto, rebased, err)
	}
	build, _, err := r.service.Build(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := build.Spec.FindPanel("p1")
	p2, _ := build.Spec.FindPanel("p2")
	if build.BaseVersion != 2 || build.Saves != 1 || p1.Panel.Queries[0].Query != "rate(errors[5m])" || p2.Panel.Title != "p2 renamed" {
		t.Fatalf("build = base %d, saves %d, p1 query %q, p2 title %q", build.BaseVersion, build.Saves, p1.Panel.Queries[0].Query, p2.Panel.Title)
	}

	_, ver, _, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil || ver.Number != 3 || ver.ParentVersion == nil || *ver.ParentVersion != 2 {
		t.Fatalf("commit after the rebase = version %d, err %v", ver.Number, err)
	}
}

func TestARebaseRefusesABuildThatChangedWhatTheNewerVersionChanged(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	before := requeried(panelsSpec("t", "p1", "p2"), "p1")
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: before}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p1", "p1 renamed"))

	if _, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt); err != nil || rebased {
		t.Fatalf("rebase = %v, err %v, want no rebase", rebased, err)
	}
	build, _, err := r.service.Build(t.Context(), id)
	if err != nil || build.BaseVersion != 1 {
		t.Fatalf("build base = %d, err %v, want the build untouched", build.BaseVersion, err)
	}
	p1, _ := build.Spec.FindPanel("p1")
	if p1.Panel.Queries[0].Query != "rate(errors[5m])" || p1.Panel.Title != "p1" {
		t.Fatalf("build p1 = %+v, want the build's own edit", p1.Panel)
	}
}

func TestARebaseRefusesABuildPanelThatFailsOnTheNewerVersion(t *testing.T) {
	r, p := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))
	p.setFail("rate(errors", true)

	if _, rebased, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt); err != nil || rebased {
		t.Fatalf("rebase = %v, err %v, want no rebase", rebased, err)
	}
}

func TestARebaseByALostAttemptWritesNothing(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "p1", "p2")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1, Spec: requeried(panelsSpec("t", "p1", "p2"), "p1"),
	}); err != nil {
		t.Fatal(err)
	}
	r.saveConcurrent(t, dash, 1, renamed(panelsSpec("t", "p1", "p2"), "p2", "p2 renamed"))

	if _, _, err := r.service.RebaseBuild(t.Context(), testUser, id, buildAttempt+1); !errors.Is(err, ErrBuildLost) {
		t.Fatalf("err = %v, want ErrBuildLost", err)
	}
	if build, _, err := r.service.Build(t.Context(), id); err != nil || build.BaseVersion != 1 {
		t.Fatalf("build base = %d, err %v, want it untouched", build.BaseVersion, err)
	}
}
