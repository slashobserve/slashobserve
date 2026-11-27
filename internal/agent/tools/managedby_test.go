package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestGetDashboardPromisesExactlyTheDeclaredManagedByValues(t *testing.T) {
	schema := compileOutput(t, nameGetDashboard, outputSchemas[nameGetDashboard])
	accepts := func(managedBy string) bool {
		body, err := json.Marshal(dashboardOut{DashboardID: "d1", Spec: &spec.Dashboard{}, ManagedBy: managedBy})
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		return schema.Validate(value) == nil
	}
	for _, managedBy := range folders.ManagedByStates.Names() {
		if !accepts(managedBy) {
			t.Errorf("the get_dashboard output schema refuses managed_by %q", managedBy)
		}
	}
	if accepts("web") {
		t.Error("the get_dashboard output schema takes a write channel as managed_by")
	}
}

func TestEveryEditAnswerOverMCPCarriesTheWarningOfWhatManagesTheDashboard(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	version := dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: 2}, Spec: &spec.Dashboard{Title: "t"}}
	warningOf := func(content any) (string, bool) {
		body, err := json.Marshal(content)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Warning *string `json:"warning"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Warning == nil {
			return "", false
		}
		return *got.Warning, true
	}
	for _, managedBy := range []folders.ManagedBy{folders.ManagedByWorkspace, folders.ManagedByFile, folders.ManagedByApply} {
		dash := dashboards.Dashboard{ID: "d1", Title: "t", ManagedBy: managedBy}
		answers := map[string]any{
			"a save":                r.saveSucceeded(t.Context(), dashboards.SaveResult{Dashboard: dash, Version: version}, nil).Content,
			"a restore or a build":  r.editedOut(dash, version, nil),
			"a Grafana update":      newImportedOut(nil, imports.Imported{Dashboard: dash, Version: version, Updated: true}),
			"a merge of a proposal": proposals.NewMergedJSON(proposals.Proposal{Warning: managedBy.EditWarning()}),
		}
		for name, content := range answers {
			got, has := warningOf(content)
			if want := managedBy.EditWarning(); got != want || has != (want != "") {
				t.Errorf("%s on a dashboard managed by %s answers warning %q (key present: %t), want %q", name, managedBy, got, has, want)
			}
		}
	}
}
