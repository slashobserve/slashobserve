package spec_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestActionReferences(t *testing.T) {
	type ref struct {
		name, format string
		start, end   int
	}
	cases := []struct {
		name     string
		template string
		want     []ref
	}{
		{"no references", "https://example.com/run", nil},
		{"braced variable", "${env}", []ref{{"env", "", 0, 6}}},
		{"bare variable", "a ${env} b $host", []ref{{"env", "", 2, 8}, {"host", "", 11, 16}}},
		{"format", "x=${name:csv}", []ref{{"name", "csv", 2, 13}}},
		{"unknown format is left to the check", "${name:nosuch}", []ref{{"name", "nosuch", 0, 14}}},
		{"time builtins", "${__from}-${__to}", []ref{{"__from", "", 0, 9}, {"__to", "", 10, 17}}},
		{"user builtin with a dot", "${__user.login}", []ref{{"__user.login", "", 0, 15}}},
		{"value builtins", "${__value.raw}${__value.text}${__value.numeric}", []ref{
			{"__value.raw", "", 0, 14}, {"__value.text", "", 14, 29}, {"__value.numeric", "", 29, 47}}},
		{"data field", "${__data.fields.status}", []ref{{"__data.fields.status", "", 0, 23}}},
		{"bare names stop at a dot", "$__user.login", []ref{{"__user", "", 0, 7}}},
		{"dollar amounts are text", "costs $5 or $ or $-", nil},
		{"unknown name is still reported", "${nope}", []ref{{"nope", "", 0, 7}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs, err := spec.ActionReferences(tc.template)
			if err != nil {
				t.Fatal(err)
			}
			var got []ref
			for _, r := range refs {
				got = append(got, ref{r.Name, r.Format, r.Start, r.End})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for _, r := range refs {
				if tc.template[r.Start] != '$' {
					t.Errorf("reference %q does not start at a dollar sign", r.Name)
				}
			}
		})
	}
}

func TestActionReferencesRejectsBrokenBraces(t *testing.T) {
	for _, template := range []string{"${", "x ${env", "${}", "${:csv}", "${name:}"} {
		t.Run(template, func(t *testing.T) {
			if _, err := spec.ActionReferences(template); err == nil {
				t.Fatalf("ActionReferences(%q) succeeded", template)
			}
		})
	}
}

func actionDashboard(t *testing.T, action map[string]any) []byte {
	t.Helper()
	action["title"] = "Run"
	if _, ok := action["url"]; !ok {
		action["url"] = "https://example.com/run"
	}
	data, err := json.Marshal(map[string]any{
		"specVersion": "0",
		"title":       "Actions",
		"time":        map[string]any{"from": "now-15m", "to": "now"},
		"variables":   []any{map[string]any{"name": "env", "type": "textbox"}},
		"panels": []any{map[string]any{
			"id": "p1", "type": "canvas", "title": "Canvas",
			"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
			"datasource": "prom", "queries": []any{map[string]any{"query": "up"}},
			"display": map[string]any{"elements": []any{map[string]any{
				"id": "b", "kind": "button", "left": 1, "top": 1, "width": 10, "height": 10,
				"actions": []any{action},
			}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

const actionPath = "/panels/0/display/elements/0/actions/0"

func TestActionChecks(t *testing.T) {
	param := func(name string) []any { return []any{map[string]any{"name": name, "value": "v"}} }
	cases := []struct {
		name   string
		action map[string]any
		code   spec.IssueCode
		path   string
	}{
		{"every builtin and a declared variable", map[string]any{
			"url":     "https://example.com/${env}/$env/${__from}/${__to:csv}/${__user.login}/${__user.email}/${__user.name}/${__user.id}",
			"body":    "${__value.raw} ${__value.text} ${__value.numeric} ${__data.fields.status} ${env:sqlstring}",
			"headers": []any{map[string]any{"name": "X-Env", "value": "${env}"}},
			"query":   []any{map[string]any{"name": "from", "value": "${__from}"}},
		}, "", ""},
		{"undeclared variable in the url", map[string]any{"url": "https://example.com/${nope}"}, spec.CodeUnknownVariable, actionPath + "/url"},
		{"undeclared bare variable in the body", map[string]any{"body": "$nope"}, spec.CodeUnknownVariable, actionPath + "/body"},
		{"undeclared variable in a header value", map[string]any{"headers": []any{map[string]any{"name": "X-A", "value": "${nope}"}}}, spec.CodeUnknownVariable, actionPath + "/headers/0/value"},
		{"undeclared variable in a query value", map[string]any{"query": []any{map[string]any{"name": "a", "value": "${nope}"}}}, spec.CodeUnknownVariable, actionPath + "/query/0/value"},
		{"bare dotted builtin is not a builtin", map[string]any{"body": "$__user.login"}, spec.CodeUnknownVariable, actionPath + "/body"},
		{"data fields needs a field", map[string]any{"body": "${__data.fields.}"}, spec.CodeUnknownVariable, actionPath + "/body"},
		{"unknown builtin", map[string]any{"body": "${__value.bogus}"}, spec.CodeUnknownVariable, actionPath + "/body"},
		{"unknown format", map[string]any{"body": "${env:nosuch}"}, spec.CodeUnknownVariable, actionPath + "/body"},
		{"unterminated reference", map[string]any{"body": "${env"}, spec.CodeInvalidReference, actionPath + "/body"},
		{"authorization header", map[string]any{"headers": param("authorization")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"header names ignore case", map[string]any{"headers": param("CoOkIe")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"content type is set by contentType", map[string]any{"headers": param("Content-Type")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"host header", map[string]any{"headers": param("Host")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"transfer encoding header", map[string]any{"headers": param("Transfer-Encoding")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"credential word inside a header name", map[string]any{"headers": param("X-Api-Key")}, spec.CodeActionHeaderNotAllowed, actionPath + "/headers/0/name"},
		{"token query parameter", map[string]any{"query": param("access_token")}, spec.CodeActionHeaderNotAllowed, actionPath + "/query/0/name"},
		{"signature query parameter", map[string]any{"query": param("Signature")}, spec.CodeActionHeaderNotAllowed, actionPath + "/query/0/name"},
		{"session query parameter", map[string]any{"query": param("sessionid")}, spec.CodeActionHeaderNotAllowed, actionPath + "/query/0/name"},
		{"plain header", map[string]any{"headers": param("X-Request-Source")}, "", ""},
		{"plain query parameter", map[string]any{"query": param("dry_run")}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Parse(actionDashboard(t, tc.action))
			assertSingleIssue(t, err, tc.code, tc.path)
		})
	}
}

func assertSingleIssue(t *testing.T, err error, code spec.IssueCode, path string) {
	t.Helper()
	if code == "" {
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		return
	}
	var ve *spec.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Parse error = %v, want *spec.ValidationError", err)
	}
	if len(ve.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(ve.Issues), ve.Issues)
	}
	if got := ve.Issues[0]; got.Code != code || got.Path != path {
		t.Fatalf("got %s at %q (%s), want %s at %q", got.Code, got.Path, got.Message, code, path)
	}
}

func TestActionHeaderMessageNamesTheRule(t *testing.T) {
	_, err := spec.Parse(actionDashboard(t, map[string]any{"headers": []any{map[string]any{"name": "Cookie", "value": "v"}}}))
	if err == nil || !strings.Contains(err.Error(), "credentials and transport headers are never stored in a dashboard; the server adds what it needs") {
		t.Fatalf("error = %v", err)
	}
}

func geomapDashboard(t *testing.T, display map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"specVersion": "0",
		"title":       "Map",
		"time":        map[string]any{"from": "now-15m", "to": "now"},
		"panels": []any{map[string]any{
			"id": "p1", "type": "geomap", "title": "Map",
			"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
			"datasource": "prom", "queries": []any{map[string]any{"query": "up"}},
			"display": display,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGeomapChecks(t *testing.T) {
	layers := []any{map[string]any{"type": "markers"}}
	cases := []struct {
		name    string
		display map[string]any
		code    spec.IssueCode
		path    string
	}{
		{"flat marker keys alone", map[string]any{"location": "coords", "latField": "lat", "lonField": "lon", "markerSize": 4}, "", ""},
		{"layers alone", map[string]any{"layers": layers}, "", ""},
		{"layers with view and basemap", map[string]any{"layers": layers, "view": "europe", "basemap": "carto"}, "", ""},
		{"layers with location", map[string]any{"layers": layers, "location": "auto"}, spec.CodeGeomapLayersConflict, "/panels/0/display/layers"},
		{"layers with geohash field", map[string]any{"layers": layers, "geohashField": "gh"}, spec.CodeGeomapLayersConflict, "/panels/0/display/layers"},
		{"layers with marker sizes", map[string]any{"layers": layers, "markerSize": 5, "markerSizeMax": 20}, spec.CodeGeomapLayersConflict, "/panels/0/display/layers"},
		{"layers with size and color fields", map[string]any{"layers": layers, "sizeField": "n", "colorField": "c"}, spec.CodeGeomapLayersConflict, "/panels/0/display/layers"},
		{"zoom limits in order", map[string]any{"minZoom": 2, "maxZoom": 8, "zoom": 5}, "", ""},
		{"equal zoom limits", map[string]any{"minZoom": 4, "maxZoom": 4, "zoom": 4}, "", ""},
		{"min zoom above max zoom", map[string]any{"minZoom": 9, "maxZoom": 3}, spec.CodeZoomOrder, "/panels/0/display/maxZoom"},
		{"zoom below the limits", map[string]any{"minZoom": 4, "maxZoom": 8, "zoom": 2}, spec.CodeZoomOrder, "/panels/0/display/zoom"},
		{"zoom above the limits", map[string]any{"minZoom": 4, "maxZoom": 8, "zoom": 10}, spec.CodeZoomOrder, "/panels/0/display/zoom"},
		{"zoom with only one limit is not compared", map[string]any{"minZoom": 4, "zoom": 2}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Parse(geomapDashboard(t, tc.display))
			assertSingleIssue(t, err, tc.code, tc.path)
		})
	}
}

func TestCanvasOneClickNeedsWhatItRuns(t *testing.T) {
	build := func(extra map[string]any) []byte {
		el := map[string]any{"id": "b", "kind": "button", "left": 1, "top": 1, "width": 10, "height": 10}
		for k, v := range extra {
			el[k] = v
		}
		data, err := json.Marshal(map[string]any{
			"specVersion": "0", "title": "C", "time": map[string]any{"from": "now-15m", "to": "now"},
			"panels": []any{map[string]any{
				"id": "p1", "type": "canvas", "title": "C",
				"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
				"datasource": "prom", "queries": []any{map[string]any{"query": "up"}},
				"display": map[string]any{"elements": []any{el}},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	link := []any{map[string]any{"title": "Docs", "url": "https://example.com"}}
	action := []any{map[string]any{"title": "Run", "url": "https://example.com/run"}}
	cases := []struct {
		name  string
		extra map[string]any
		code  spec.IssueCode
	}{
		{"action with an action", map[string]any{"oneClick": "action", "actions": action}, ""},
		{"link with a link", map[string]any{"oneClick": "link", "links": link}, ""},
		{"action without actions", map[string]any{"oneClick": "action", "links": link}, spec.CodeInvalidReference},
		{"link without links", map[string]any{"oneClick": "link", "actions": action}, spec.CodeInvalidReference},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Parse(build(tc.extra))
			assertSingleIssue(t, err, tc.code, "/panels/0/display/elements/0/oneClick")
		})
	}
}

func TestGeomapRuleValueIsAStringOrANumber(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
		valid bool
	}{
		{"string", "France", `"France"`, true},
		{"integer", 3, `3`, true},
		{"fraction", 2.5, `2.5`, true},
		{"bool", true, "", false},
		{"null", nil, "", false},
		{"list", []any{1}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layer := map[string]any{"type": "geojson", "rules": []any{map[string]any{"property": "name", "operation": "eq", "value": tc.value}}}
			d, err := spec.Parse(geomapDashboard(t, map[string]any{"layers": []any{layer}}))
			if !tc.valid {
				if err == nil {
					t.Fatal("Parse accepted the rule value")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(d.Panels[0].Display.Layers[0].Rules[0].Value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("value marshals as %s, want %s", got, tc.want)
			}
		})
	}
}
