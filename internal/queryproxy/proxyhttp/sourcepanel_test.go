package proxyhttp

import (
	"net/http"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func reusingDashboard() *spec.Dashboard {
	d := testDashboard()
	d.Panels = append(d.Panels,
		spec.Panel{ID: "p_reuse", Type: spec.Table, Title: "reuse", Queries: []spec.Query{{Source: &spec.DashboardSource{Panel: "p_ts", WithTransforms: true}}}},
		spec.Panel{ID: "p_reuse_hidden", Type: spec.Table, Title: "hidden", Queries: []spec.Query{{Hide: true, Source: &spec.DashboardSource{Panel: "p_ts"}}}},
		spec.Panel{ID: "p_reuse_row", Type: spec.Row, Panels: []spec.Panel{
			{ID: "p_reuse_child", Type: spec.Stat, Title: "child", Queries: []spec.Query{{Source: &spec.DashboardSource{Panel: "p_child"}}}},
		}},
	)
	return d
}

func newReusingHarness(t *testing.T) *harness {
	t.Helper()
	hn := newHarness(t)
	hn.specs.known[specKey{id: hn.id, version: 1}] = reusingDashboard()
	return hn
}

func TestPanelThatReusesAnotherReturnsItsFramesFromOneSharedRun(t *testing.T) {
	hn := newReusingHarness(t)
	query := window + "&var-job=api"

	source := decodeData(t, hn.get(hn.dataPath("p_ts", query), ""))
	reuse := decodeData(t, hn.get(hn.dataPath("p_reuse", query), ""))

	if source.Meta.SourcePanel != "" {
		t.Fatalf("the source panel reports source_panel %q", source.Meta.SourcePanel)
	}
	if reuse.Meta.SourcePanel != "p_ts" {
		t.Fatalf("source_panel = %q, want p_ts", reuse.Meta.SourcePanel)
	}
	if string(reuse.Frames) != string(source.Frames) {
		t.Fatalf("frames = %s, want the source's %s", reuse.Frames, source.Frames)
	}
	if !reuse.Meta.Cached {
		t.Fatal("the reusing panel did not hit the source panel's cache entry")
	}
	if reuse.Meta.Kind != source.Meta.Kind || reuse.Meta.StepMS != source.Meta.StepMS || reuse.Meta.ExecutedAt != source.Meta.ExecutedAt {
		t.Fatalf("meta = %+v, want the source's %+v", reuse.Meta, source.Meta)
	}
	if ranges, instants, _, _ := hn.ds.counts(); ranges != 1 || instants != 0 {
		t.Fatalf("%d range and %d instant queries, want 1 and 0", ranges, instants)
	}
}

func TestPanelThatReusesAnotherRunsItFirstWhenItComesFirst(t *testing.T) {
	hn := newReusingHarness(t)
	query := window + "&var-job=api"

	reuse := decodeData(t, hn.get(hn.dataPath("p_reuse", query), ""))
	source := decodeData(t, hn.get(hn.dataPath("p_ts", query), ""))

	if reuse.Meta.Cached || reuse.Meta.SourcePanel != "p_ts" {
		t.Fatalf("meta = %+v, want an uncached run for p_ts", reuse.Meta)
	}
	if !source.Meta.Cached || string(source.Frames) != string(reuse.Frames) {
		t.Fatalf("the source panel did not reuse the run: %+v", source.Meta)
	}
	if ranges, _, _, _ := hn.ds.counts(); ranges != 1 {
		t.Fatalf("%d range queries, want 1", ranges)
	}
}

func TestPanelThatReusesAnotherUsesTheSourcesKindAndRowChildren(t *testing.T) {
	hn := newReusingHarness(t)

	body := decodeData(t, hn.get(hn.dataPath("p_reuse_child", window), ""))

	if body.Meta.Kind != queryproxy.KindInstant || body.Meta.SourcePanel != "p_child" {
		t.Fatalf("meta = %+v, want an instant run of p_child", body.Meta)
	}
}

func TestHiddenReusingQueryReturnsNoFramesAndRunsNothing(t *testing.T) {
	hn := newReusingHarness(t)

	body := decodeData(t, hn.get(hn.dataPath("p_reuse_hidden", window+"&var-job=api"), ""))

	if string(body.Frames) != "[]" || len(body.Meta.Queries) != 0 || body.Meta.SourcePanel != "p_ts" || body.Meta.Kind != queryproxy.KindRange {
		t.Fatalf("frames %s, meta %+v; want no frames and no queries from a range panel", body.Frames, body.Meta)
	}
	if ranges, instants, _, _ := hn.ds.counts(); ranges != 0 || instants != 0 {
		t.Fatalf("%d range and %d instant queries, want none", ranges, instants)
	}
}

func TestPreviewRefusesAPanelThatReusesAnother(t *testing.T) {
	hn := newPreviewHarness(t)
	saved := previewDashboard()
	saved.Panels = append(saved.Panels, spec.Panel{ID: "p_other", Type: spec.Timeseries, Title: "other", Datasource: "prom", GridPos: spec.GridPos{X: 12, W: 12, H: 8}, Queries: []spec.Query{{Query: "up"}}})
	hn.specs.known[specKey{id: hn.id, version: 1}] = saved

	rec := hn.previewAs(auth.KindUser, "prom", func(p *spec.Panel) {
		p.Datasource = ""
		p.Queries = []spec.Query{{Source: &spec.DashboardSource{Panel: "p_other"}}}
	})

	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if ranges, _, _, _ := hn.ds.counts(); ranges != 0 {
		t.Fatalf("the preview ran %d queries", ranges)
	}
}
