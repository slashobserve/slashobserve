package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
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

func TestASaveResultCarriesTheWarningOfTheSave(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	saved := dashboards.SaveResult{
		Dashboard: dashboards.Dashboard{ID: "d1", Title: "t"},
		Version:   dashboards.Version{VersionMeta: dashboards.VersionMeta{Number: 2}, Spec: &spec.Dashboard{Title: "t"}},
	}
	fields := func(saved dashboards.SaveResult) map[string]json.RawMessage {
		body, err := json.Marshal(r.saveSucceeded(t.Context(), saved, nil).Content)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if _, ok := fields(saved)["warning"]; ok {
		t.Error("a save with nothing to warn about has a warning key")
	}
	saved.Warning = folders.ManagedByFile.EditWarning()
	var warning string
	if err := json.Unmarshal(fields(saved)["warning"], &warning); err != nil || warning != folders.ManagedByFile.EditWarning() {
		t.Errorf("warning = %q (%v), want the file warning", warning, err)
	}
}
