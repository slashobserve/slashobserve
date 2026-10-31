package grafana

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func panelWithLinks(links string) string {
	return panelDoc("graph", upTarget+`,"links":[`+links+`]`)
}

func TestPanelLinkToADashboardIsAByTitleAddress(t *testing.T) {
	cases := []struct {
		name  string
		link  string
		title string
		url   string
		tab   bool
	}{
		{
			"time range and variables",
			`{"type":"dashboard","dashboard":"Thanos / Query","title":"Thanos / Query","keepTime":true,"includeVars":true}`,
			"Thanos / Query",
			"/dashboards/by-title?title=Thanos+%2F+Query&${__url_time_range}&${__all_variables}",
			false,
		},
		{
			"time range only",
			`{"type":"dashboard","dashboard":"Query","title":"Go","keepTime":true}`,
			"Go",
			"/dashboards/by-title?title=Query&${__url_time_range}",
			false,
		},
		{
			"variables only",
			`{"type":"dashboard","dashboard":"Query","title":"Go","includeVars":true}`,
			"Go",
			"/dashboards/by-title?title=Query&${__all_variables}",
			false,
		},
		{
			"neither, in a new tab, titled after the dashboard",
			`{"type":"dashboard","dashboard":"Query","targetBlank":true}`,
			"Query",
			"/dashboards/by-title?title=Query",
			true,
		},
		{
			"a title with reserved characters",
			`{"type":"dashboard","dashboard":"A&B=C#D $x ${y}","title":"t"}`,
			"t",
			"/dashboards/by-title?title=A%26B%3DC%23D+%24x+%24%7By%7D",
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelWithLinks(tc.link))
			want := []spec.Link{{Title: tc.title, URL: tc.url, NewTab: tc.tab}}
			if len(panel.Links) != 1 || panel.Links[0] != want[0] || !clean(line) {
				t.Fatalf("links = %+v, want %+v, line = %+v", panel.Links, want, line)
			}
		})
	}
}

func TestPanelLinkToADashboardWithoutATitleIsReported(t *testing.T) {
	for _, link := range []string{
		`{"type":"dashboard","title":"Query","keepTime":true}`,
		`{"type":"dashboard","dashboard":"   ","title":"Query"}`,
		`{"type":"dashboard","dashboard":"` + strings.Repeat("x", 201) + `"}`,
	} {
		panel, line := convertedPanel(t, panelWithLinks(link))
		if len(panel.Links) != 0 || line.Status != statusPartial || len(line.Paths) != 1 || line.Paths[0] != "links[0]" {
			t.Fatalf("link %s: links = %+v, line = %+v", link, panel.Links, line)
		}
	}
}

func TestPanelLinkOfAnotherTypeIsReported(t *testing.T) {
	panel, line := convertedPanel(t, panelWithLinks(`{"type":"absolute","url":"https://example.com","title":"x"}`))
	if len(panel.Links) != 0 || line.Status != statusPartial {
		t.Fatalf("links = %+v, line = %+v", panel.Links, line)
	}
}

func TestPlainPanelLinkCarriesTimeAndVariablesInItsAddress(t *testing.T) {
	cases := []struct {
		name, link, url string
	}{
		{
			"no query yet",
			`{"url":"https://example.com/d","title":"t","keepTime":true,"includeVars":true}`,
			"https://example.com/d?${__url_time_range}&${__all_variables}",
		},
		{
			"an existing query",
			`{"url":"https://example.com/d?a=1","title":"t","includeVars":true}`,
			"https://example.com/d?a=1&${__all_variables}",
		},
		{
			"a query that ends in an ampersand",
			`{"url":"https://example.com/d?a=1&","title":"t","keepTime":true}`,
			"https://example.com/d?a=1&${__url_time_range}",
		},
		{
			"a fragment stays last",
			`{"url":"https://example.com/d#top","title":"t","keepTime":true}`,
			"https://example.com/d?${__url_time_range}#top",
		},
		{
			"neither option leaves the address alone",
			`{"url":"https://example.com/d","title":"t","keepTime":false,"includeVars":false}`,
			"https://example.com/d",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelWithLinks(tc.link))
			if len(panel.Links) != 1 || panel.Links[0].URL != tc.url || !clean(line) {
				t.Fatalf("links = %+v, want url %q, line = %+v", panel.Links, tc.url, line)
			}
		})
	}
}

func TestPanelLinkAddressIsCappedAfterTheTokensAreAdded(t *testing.T) {
	fits := "https://example.com/" + strings.Repeat("a", 1980)
	cases := []struct {
		name, link string
		kept       bool
	}{
		{"plain address at the cap", `{"url":"` + fits + `","title":"t"}`, true},
		{"plain address pushed past the cap by the time range", `{"url":"` + fits + `","title":"t","keepTime":true}`, false},
		{"plain address pushed past the cap by the variables", `{"url":"` + fits + `","title":"t","includeVars":true}`, false},
		{"dashboard title escaped past the cap", `{"type":"dashboard","dashboard":"` + strings.Repeat("😀", 200) + `","keepTime":true,"includeVars":true}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := convertedPanel(t, panelWithLinks(tc.link))
			if tc.kept {
				if len(panel.Links) != 1 || !clean(line) {
					t.Fatalf("links = %d, line = %+v", len(panel.Links), line)
				}
				return
			}
			if len(panel.Links) != 0 || line.Status != statusPartial || line.Paths[0] != "links[0]" {
				t.Fatalf("links = %d, line = %+v", len(panel.Links), line)
			}
		})
	}
}

func TestLinkOptionsOnAFieldLinkAreCarriedToo(t *testing.T) {
	doc := panelDoc("stat", upTarget+`,"fieldConfig":{"defaults":{"links":[{"title":"Open","url":"https://example.com/d","keepTime":true}]},"overrides":[]}`)
	panel, line := convertedPanel(t, doc)
	if len(panel.Links) != 1 || panel.Links[0].URL != "https://example.com/d?${__url_time_range}" || !clean(line) {
		t.Fatalf("links = %+v, line = %+v", panel.Links, line)
	}
}
