package dashboards

import (
	"fmt"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func reuseDashboard(t *testing.T, variables, sourceQuery, sourceID string) *spec.Dashboard {
	t.Helper()
	raw := fmt.Sprintf(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"variables":[%s],"panels":[`+
		`{"id":"src","type":"stat","title":"Src","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":%q}]},`+
		`{"id":"other","type":"stat","title":"Other","gridPos":{"x":6,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"sum(up)"}]},`+
		`{"id":"dep","type":"table","title":"Dep","gridPos":{"x":12,"y":0,"w":6,"h":4},"queries":[{"source":{"panel":%q}}]}]}`, variables, sourceQuery, sourceID)
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse test dashboard: %v", err)
	}
	return d
}

func TestCarryUnverifiedFollowsThePanelAReusingPanelReuses(t *testing.T) {
	const inst = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(nope, instance)"}`
	const edited = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(other, instance)"}`
	query := `sum(up{instance=~"$inst"})`
	base := reuseDashboard(t, inst, query, "src")
	prev := Unverified{Panels: []UnverifiedPanel{{PanelID: "dep", Issues: []spec.Issue{issueAt("/panels/2/queries/0/source/panel")}}}}
	fails := []spec.Issue{issueAt("/panels/2/queries/0/source/panel")}

	if carried, blocking := CarryUnverified(base, prev, base, fails); len(blocking) != 0 || len(carried.Panels) != 1 {
		t.Fatalf("unchanged: carried %+v blocking %v, want carried", carried, blocking)
	}

	changes := map[string]*spec.Dashboard{
		"the source's query":            reuseDashboard(t, inst, `sum(up{instance=~"$inst"}) + 1`, "src"),
		"a variable the source reaches": reuseDashboard(t, edited, query, "src"),
		"the panel it reuses":           reuseDashboard(t, inst, query, "other"),
	}
	for name, next := range changes {
		carried, blocking := CarryUnverified(base, prev, next, fails)
		if len(blocking) != 1 || !carried.Empty() {
			t.Errorf("%s changed: carried %+v blocking %v, want the issue blocking", name, carried, blocking)
		}
	}
}
