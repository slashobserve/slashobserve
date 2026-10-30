package queryproxy

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func reusingPanel(id, source string) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Table, Title: id, Queries: []spec.Query{{Source: &spec.DashboardSource{Panel: source}}}}
}

func reusingDashboard(source spec.Panel) *spec.Dashboard {
	return dashboardOf([]spec.Panel{source, reusingPanel("reuse", source.ID)})
}

func TestCheckSpecRunsASourcePanelOnceForItsDependent(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})

	issues, err := runner.CheckSpec(t.Context(), reusingDashboard(panelOf("src", spec.Timeseries, "prom", "up")), testNow, datasource.QueryUser{})

	if err != nil || issues != nil {
		t.Fatalf("CheckSpec = %+v, %v; want a clean check", issues, err)
	}
	if got := fake.callCount(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1: %+v", got, fake.recorded())
	}
}

func TestCheckSpecFailsTheDependentWhenItsSourceFails(t *testing.T) {
	fake := newFake("prometheus")
	fake.onRange = failWith(errors.New("boom"))
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})

	issues, err := runner.CheckSpec(t.Context(), reusingDashboard(panelOf("src", spec.Timeseries, "prom", "up")), testNow, datasource.QueryUser{})

	if err != nil {
		t.Fatalf("CheckSpec error: %v", err)
	}
	assertIssues(t, issues, []wantIssue{
		{"/panels/0/queries/0/query", CodeInternal, ""},
		{"/panels/1/queries/0/source/panel", CodeInternal, ""},
	})
}

func TestCheckSpecFailsTheDependentWhenItsSourceDatasourceIsUnknown(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})

	issues, err := runner.CheckSpec(t.Context(), reusingDashboard(panelOf("src", spec.Timeseries, "missing", "up")), testNow, datasource.QueryUser{})

	if err != nil {
		t.Fatalf("CheckSpec error: %v", err)
	}
	assertIssues(t, issues, []wantIssue{
		{"/panels/0/datasource", CodeUnknownDatasource, ""},
		{"/panels/1/queries/0/source/panel", CodeUnknownDatasource, ""},
	})
}

func TestCheckPanelsOfADependentChecksItsSource(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})
	d := reusingDashboard(panelOf("src", spec.Timeseries, "prom", "up"))

	issues, err := runner.CheckPanels(t.Context(), d, testNow, datasource.QueryUser{}, []string{"reuse"})

	if err != nil || issues != nil {
		t.Fatalf("CheckPanels = %+v, %v; want a clean check", issues, err)
	}
	if got := fake.callCount(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (the source's)", got)
	}
}

func TestCheckDatasourcesIgnoresAPanelThatReusesAnother(t *testing.T) {
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": newFake("prometheus")})

	if got := runner.CheckDatasources(reusingDashboard(panelOf("src", spec.Timeseries, "prom", "up"))); got != nil {
		t.Fatalf("issues = %+v, want none", got)
	}
}

func TestRunPanelRefusesAPanelThatReusesAnother(t *testing.T) {
	fake := newFake("prometheus")
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"prom": fake})

	_, err := runner.RunPanel(t.Context(), Request{
		Dashboard: reusingDashboard(panelOf("src", spec.Timeseries, "prom", "up")),
		PanelID:   "reuse",
		Range:     testRange(),
	})

	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("RunPanel error = %v, want ErrInvalidRequest", err)
	}
	if fake.callCount() != 0 {
		t.Fatalf("upstream calls = %d, want 0", fake.callCount())
	}
}
