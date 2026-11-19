package spec_test

import (
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func linkDashboard(t *testing.T, mutate func(dash, panel map[string]any)) []byte {
	t.Helper()
	panel := map[string]any{
		"id": "p1", "type": "canvas", "title": "Canvas",
		"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
		"datasource": "prom", "queries": []any{map[string]any{"query": "up"}},
		"display": map[string]any{"elements": []any{map[string]any{
			"id": "b", "kind": "button", "left": 1, "top": 1, "width": 10, "height": 10,
		}}},
	}
	dash := map[string]any{
		"specVersion": "0",
		"title":       "Links",
		"time":        map[string]any{"from": "now-15m", "to": "now"},
		"variables":   []any{map[string]any{"name": "search", "type": "textbox"}},
		"panels":      []any{panel},
	}
	mutate(dash, panel)
	data, err := json.Marshal(dash)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLinkSchemeChecks(t *testing.T) {
	link := func(url string) []any { return []any{map[string]any{"title": "Go", "url": url}} }
	dashboardLink := func(url string) []any {
		return []any{map[string]any{"title": "Go", "url": url}}
	}
	cases := []struct {
		name   string
		mutate func(dash, panel map[string]any)
		code   spec.IssueCode
		path   string
	}{
		{"https dashboard link", func(d, p map[string]any) { d["links"] = dashboardLink("https://example.com/a") }, "", ""},
		{"mailto panel link", func(d, p map[string]any) { p["links"] = link("mailto:oncall@example.com") }, "", ""},
		{"relative and anchor links", func(d, p map[string]any) { p["links"] = append(link("/d/abc"), link("#top")[0], link("?x=1")[0]) }, "", ""},
		{"variable as the whole address", func(d, p map[string]any) { p["links"] = link("${search}") }, "", ""},
		{"variable after an https prefix", func(d, p map[string]any) { p["links"] = link("https://example.com/?q=${search}") }, "", ""},
		{"scheme case is ignored", func(d, p map[string]any) { p["links"] = link("HTTPS://example.com") }, "", ""},
		{"javascript dashboard link", func(d, p map[string]any) { d["links"] = dashboardLink("javascript:alert(1)") }, spec.CodeUnsafeLink, "/links/0/url"},
		{"javascript panel link", func(d, p map[string]any) { p["links"] = link("javascript:alert(1)") }, spec.CodeUnsafeLink, "/panels/0/links/0/url"},
		{"data url in an override link", func(d, p map[string]any) {
			p["overrides"] = []any{map[string]any{"match": map[string]any{"series": "x"}, "links": link("data:text/html,hi")}}
		}, spec.CodeUnsafeLink, "/panels/0/overrides/0/links/0/url"},
		{"vbscript canvas element link", func(d, p map[string]any) {
			elements := p["display"].(map[string]any)["elements"].([]any)
			elements[0].(map[string]any)["links"] = link("vbscript:x")
		}, spec.CodeUnsafeLink, "/panels/0/display/elements/0/links/0/url"},
		{"mixed case scheme", func(d, p map[string]any) { p["links"] = link("JaVaScRiPt:alert(1)") }, spec.CodeUnsafeLink, "/panels/0/links/0/url"},
		{"leading space", func(d, p map[string]any) { p["links"] = link("  javascript:alert(1)") }, spec.CodeUnsafeLink, "/panels/0/links/0/url"},
		{"tab inside the scheme", func(d, p map[string]any) { p["links"] = link("java\tscript:alert(1)") }, spec.CodeUnsafeLink, "/panels/0/links/0/url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := spec.Parse(linkDashboard(t, tc.mutate))
			assertSingleIssue(t, err, tc.code, tc.path)
		})
	}
}
