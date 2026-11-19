package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func schemaHas(r *Registry, tool, field string) bool {
	for _, name := range r.fields[tool].mcp {
		if name == field {
			return true
		}
	}
	return false
}

func TestEveryToolAcceptsTheAliasOfItsDatasourceDashboardAndRunFields(t *testing.T) {
	r := New(Deps{})
	for _, tool := range r.List() {
		for _, c := range []struct {
			has, alias string
			applies    bool
		}{
			{"datasource", "datasource_uid", true},
			{"dashboard_id", "id", hasWord(tool.Name, "dashboard") && !schemaHas(r, tool.Name, "id")},
			{"run_id", "id", hasWord(tool.Name, "run") && !schemaHas(r, tool.Name, "id")},
			{"id", "run_id", hasWord(tool.Name, "run") && !schemaHas(r, tool.Name, "run_id")},
		} {
			if !c.applies || !schemaHas(r, tool.Name, c.has) {
				continue
			}
			sent := json.RawMessage(`{"` + c.alias + `":"x"}`)
			got, res, ok := r.canonicalInput(tool.Name, sent)
			if !ok {
				t.Errorf("%s refused %s: %#v", tool.Name, c.alias, res.Content)
				continue
			}
			var fields map[string]string
			if err := json.Unmarshal(got, &fields); err != nil || fields[c.has] != "x" || len(fields) != 1 {
				t.Errorf("%s: %s became %s, want %s", tool.Name, c.alias, got, c.has)
			}
		}
	}
}

func hasWord(name, word string) bool {
	for _, w := range strings.Split(name, "_") {
		if w == word {
			return true
		}
	}
	return false
}

func TestAliasesNamedInTheBugReports(t *testing.T) {
	r := New(Deps{})
	for _, c := range []struct{ tool, sent, want string }{
		{nameQueryRange, `{"datasource_uid":"prom","query":"up"}`, `{"datasource":"prom","query":"up"}`},
		{nameDiffDashboardVersions, `{"dashboard_id":"d","from_version":1,"to_version":2}`, `{"dashboard_id":"d","from":1,"to":2}`},
		{nameGetPanels, `{"dashboard_id":"d","panel_ids":["a"]}`, `{"dashboard_id":"d","ids":["a"]}`},
		{nameGetRun, `{"run_id":"r"}`, `{"id":"r"}`},
		{nameDraftQuery, `{"datasource_uid":"loki","question":"q"}`, `{"datasource":"loki","question":"q"}`},
		{nameGetDashboard, `{"id":"d"}`, `{"dashboard_id":"d"}`},
		{nameGetObservation, `{"id":"o"}`, `{"observation_id":"o"}`},
	} {
		got, res, ok := r.canonicalInput(c.tool, json.RawMessage(c.sent))
		if !ok {
			t.Errorf("%s refused %s: %#v", c.tool, c.sent, res.Content)
			continue
		}
		if !sameJSONText(t, string(got), c.want) {
			t.Errorf("%s: %s became %s, want %s", c.tool, c.sent, got, c.want)
		}
	}
}

func sameJSONText(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(ax) == string(by)
}

func TestAnAliasAndItsDocumentedNameMustAgree(t *testing.T) {
	r := New(Deps{})
	if _, _, ok := r.canonicalInput(nameQueryRange, json.RawMessage(`{"datasource":"a","datasource_uid":"a","query":"up"}`)); !ok {
		t.Error("the same value under both names was refused")
	}
	_, res, ok := r.canonicalInput(nameQueryRange, json.RawMessage(`{"datasource":"a","datasource_uid":"b","query":"up"}`))
	body, isError := res.Content.(errorBody)
	if ok || !isError || body.Code != codeInvalidRequest || !strings.Contains(body.Message, `"datasource" and "datasource_uid"`) {
		t.Errorf("two different values: ok %v, %#v", ok, res.Content)
	}
}

func TestInputThatIsNotAnObjectPassesThroughForTheHandlerToRefuse(t *testing.T) {
	r := New(Deps{})
	for _, raw := range []string{``, `null`, `[]`, `7`, `{"datasource_uid"`, `{} {}`} {
		got, _, ok := r.canonicalInput(nameQueryRange, json.RawMessage(raw))
		if !ok || string(got) != raw {
			t.Errorf("%q became %q (ok %v)", raw, got, ok)
		}
	}
}
