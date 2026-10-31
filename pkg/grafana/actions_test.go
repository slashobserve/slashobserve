package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const restartFetch = `{"method":"POST","url":"https://example.com/restart"}`

func fieldActionEntry(extra string) string {
	return `{"type":"fetch","title":"Restart",` + extra + `"fetch":` + restartFetch + `}`
}

func actionPanel(t *testing.T, typ, defaults, overrides string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc(typ, upTarget+`,"fieldConfig":{"defaults":{`+defaults+`},"overrides":[`+overrides+`]}`))
}

func TestCanvasActionInputsAreKeptOrReported(t *testing.T) {
	cases := []struct {
		name      string
		variables string
		want      []spec.ActionVariable
		reported  string
	}{
		{"kept", `[{"key":"pod","name":"Pod","type":"string"}]`, []spec.ActionVariable{{Key: "pod", Name: "Pod"}}, ""},
		{"name defaults to the key", `[{"key":"pod"}]`, []spec.ActionVariable{{Key: "pod", Name: "pod"}}, ""},
		{"another type", `[{"key":"pod","name":"Pod","type":"number"}]`, nil, "variables[0]"},
		{"bad key", `[{"key":"9pod","name":"Pod"},{"key":"ok","name":"Ok"}]`, []spec.ActionVariable{{Key: "ok", Name: "Ok"}}, "variables[0]"},
		{"repeated key", `[{"key":"pod","name":"A"},{"key":"pod","name":"B"}]`, []spec.ActionVariable{{Key: "pod", Name: "A"}}, "variables[1]"},
		{"name over 100 characters", `[{"key":"pod","name":"` + strings.Repeat("n", 101) + `"}]`, nil, "variables[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el, line := oneCanvasElement(t, actionElement(`{"type":"fetch","title":"Restart","variables":`+tc.variables+`,"fetch":`+restartFetch+`}`))
			if len(el.Actions) != 1 || !slices.Equal(el.Actions[0].Variables, tc.want) {
				t.Fatalf("actions = %+v", el.Actions)
			}
			if tc.reported == "" {
				if line.Status != statusImported {
					t.Fatalf("paths %v", line.Paths)
				}
				return
			}
			path := "options.root.elements[0].actions[0]." + tc.reported
			if !slices.Contains(line.Paths, path) || !slices.Contains(line.Reasons, actionInputReason) {
				t.Fatalf("paths %v reasons %v, want %s", line.Paths, line.Reasons, path)
			}
		})
	}
}

func TestCanvasActionInputCanBeUsedInTheAddress(t *testing.T) {
	el, line := oneCanvasElement(t, actionElement(`{"type":"fetch","title":"Restart","variables":[{"key":"pod","name":"Pod"}],"fetch":{"method":"POST","url":"https://example.com/restart/${pod}"}}`))
	if len(el.Actions) != 1 || el.Actions[0].URL != "https://example.com/restart/${pod}" || line.Status != statusImported {
		t.Fatalf("actions = %+v paths %v", el.Actions, line.Paths)
	}
}

func TestCanvasActionStyleIsReportedAndFieldActionColorImports(t *testing.T) {
	el, line := oneCanvasElement(t, actionElement(`{"type":"fetch","title":"Restart","style":{"backgroundColor":"#ff0000"},"fetch":`+restartFetch+`}`))
	if len(el.Actions) != 1 || el.Actions[0].Color != "" {
		t.Fatalf("actions = %+v", el.Actions)
	}
	if !slices.Contains(line.Paths, "options.root.elements[0].actions[0].style.backgroundColor") {
		t.Fatalf("paths %v", line.Paths)
	}

	panel, line := actionPanel(t, "stat", `"actions":[`+fieldActionEntry(`"style":{"backgroundColor":"#ff0000"},`)+`]`, "")
	if len(panel.Actions) != 1 || panel.Actions[0].Color != "#ff0000" || !clean(line) {
		t.Fatalf("actions = %+v line = %+v", panel.Actions, line)
	}

	panel, line = actionPanel(t, "stat", `"actions":[`+fieldActionEntry(`"style":{"backgroundColor":"not a color"},`)+`]`, "")
	if len(panel.Actions) != 1 || panel.Actions[0].Color != "" || !slices.Contains(line.Paths, "fieldConfig.defaults.actions[0].style.backgroundColor") {
		t.Fatalf("actions = %+v paths %v", panel.Actions, line.Paths)
	}
}

func TestActionKeysTheImporterDoesNotReadAreReported(t *testing.T) {
	_, line := oneCanvasElement(t, actionElement(`{"type":"fetch","title":"Restart","mystery":true,"fetch":`+restartFetch+`}`))
	if !slices.Contains(line.Paths, "options.root.elements[0].actions[0].mystery") {
		t.Errorf("canvas paths %v", line.Paths)
	}
	_, line = actionPanel(t, "stat", `"actions":[`+fieldActionEntry(`"mystery":true,`)+`]`, "")
	if !slices.Contains(line.Paths, "fieldConfig.defaults.actions[0].mystery") {
		t.Errorf("field paths %v", line.Paths)
	}
}

func TestFieldActionsImportOnTheTypesThatDrawThem(t *testing.T) {
	for _, typ := range []string{"stat", "gauge", "bargauge", "table"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := actionPanel(t, typ, `"actions":[`+fieldActionEntry(`"confirmation":"Sure?",`)+`]`, "")
			if len(panel.Actions) != 1 || panel.Actions[0].Confirm != "Sure?" || panel.Actions[0].URL != "https://example.com/restart" || !clean(line) {
				t.Fatalf("actions = %+v line = %+v", panel.Actions, line)
			}
		})
	}
}

func TestFieldActionsInAnOverrideImport(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"up"},"properties":[{"id":"actions","value":[` + fieldActionEntry("") + `]}]}`
	panel, line := actionPanel(t, "stat", "", override)
	got := onlyOverride(t, panel)
	if len(got.Actions) != 1 || got.Actions[0].Title != "Restart" || !clean(line) {
		t.Fatalf("override = %+v line = %+v", got, line)
	}
}

func TestFieldActionsAreReportedOnTypesThatDrawNone(t *testing.T) {
	panel, line := actionPanel(t, "timeseries", `"actions":[`+fieldActionEntry("")+`]`, "")
	if len(panel.Actions) != 0 || line.Status != statusPartial || !slices.Contains(line.Paths, "fieldConfig.defaults.actions[0].fetch.url") {
		t.Fatalf("actions = %+v status %s paths %v", panel.Actions, line.Status, line.Paths)
	}
}

func TestFieldActionsOtherThanFetchAreReported(t *testing.T) {
	panel, line := actionPanel(t, "stat", `"actions":[{"type":"infinity","title":"Query","infinity":{"method":"GET","url":"https://example.com/q"}}]`, "")
	if len(panel.Actions) != 0 || !slices.Contains(line.Reasons, decisionInfinity.reason) {
		t.Fatalf("actions = %+v reasons %v", panel.Actions, line.Reasons)
	}
}

func TestFieldKeepsTenActions(t *testing.T) {
	entries := strings.TrimSuffix(strings.Repeat(fieldActionEntry("")+",", 11), ",")
	panel, line := actionPanel(t, "stat", `"actions":[`+entries+`]`, "")
	if len(panel.Actions) != fieldActionMax || !slices.Contains(line.Reasons, fieldActionLimitReason) {
		t.Fatalf("actions = %d reasons %v", len(panel.Actions), line.Reasons)
	}
}

func TestOneClickActionIsKeptOnlyWhenItIsTheFieldsOnlyTarget(t *testing.T) {
	link := `{"title":"Runbook","url":"https://example.com/runbook"}`
	oneClickLink := `{"title":"Runbook","url":"https://example.com/runbook","oneClick":true}`
	cases := []struct {
		name         string
		members      string
		wantOneClick bool
		reported     bool
	}{
		{"alone", `"actions":[` + fieldActionEntry(`"oneClick":true,`) + `]`, true, false},
		{"beside a data link", `"links":[` + oneClickLink + `],"actions":[` + fieldActionEntry(`"oneClick":true,`) + `]`, false, true},
		{"beside another action", `"actions":[` + fieldActionEntry(`"oneClick":true,`) + `,` + fieldActionEntry("") + `]`, false, true},
		{"beside a header link", `"links":[` + link + `],"actions":[` + fieldActionEntry(`"oneClick":true,`) + `]`, true, false},
		{"not asked for", `"actions":[` + fieldActionEntry("") + `,` + fieldActionEntry("") + `]`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := actionPanel(t, "stat", tc.members, "")
			clicked := slices.ContainsFunc(panel.Actions, func(a spec.Action) bool { return a.OneClick })
			if clicked != tc.wantOneClick || (tc.reported && slices.ContainsFunc(panel.Links, func(l spec.Link) bool { return l.OneClick })) {
				t.Fatalf("actions = %+v links = %+v", panel.Actions, panel.Links)
			}
			if reported := slices.Contains(line.Reasons, oneClickTargetReason); reported != tc.reported {
				t.Fatalf("reasons %v, want reported = %v", line.Reasons, tc.reported)
			}
		})
	}
}

func TestOneClickIsSettledWhenAnOverrideListsActionsBeforeLinks(t *testing.T) {
	override := `{"matcher":{"id":"byName","options":"up"},"properties":[` +
		`{"id":"actions","value":[` + fieldActionEntry(`"oneClick":true,`) + `]},` +
		`{"id":"links","value":[{"title":"Open","url":"https://example.com/${__value.raw}","oneClick":true}]}]}`
	panel, line := actionPanel(t, "stat", "", override)
	got := onlyOverride(t, panel)
	if got.Actions[0].OneClick || got.Links[0].OneClick || !slices.Contains(line.Reasons, oneClickTargetReason) {
		t.Fatalf("override = %+v reasons %v", got, line.Reasons)
	}
}
