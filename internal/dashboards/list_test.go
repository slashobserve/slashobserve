package dashboards

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	otherUserID  = "22222222-2222-4222-8222-222222222222"
	serviceKeyID = "33333333-3333-4333-8333-333333333333"
)

var (
	otherUser  = auth.Principal{Kind: auth.KindUser, ID: otherUserID, Name: "Bob", Email: "bob@example.com"}
	serviceKey = auth.Principal{Kind: auth.KindServiceAccount, ID: serviceKeyID, Name: "ci"}
)

func specWithDatasources(title, description string, datasources ...string) []byte {
	panels := make([]string, len(datasources))
	for i, ds := range datasources {
		panels[i] = fmt.Sprintf(`{"id":"p%d","type":"timeseries","title":"p%d","gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":%q,"queries":[{"query":"up"}]}`, i, i, i*8, ds)
	}
	return fmt.Appendf(nil, `{"specVersion":"0","title":%q,"description":%q,"time":{"from":"now-15m","to":"now"},"panels":[%s]}`, title, description, strings.Join(panels, ","))
}

type listSeed struct {
	title string
	by    auth.Principal
	from  origin.Origin
	desc  string
	uids  []string
}

func (r *rig) seedList(t *testing.T) map[string]string {
	t.Helper()
	seeds := []listSeed{
		{"Checkout Latency", testUser, origin.Origin{Kind: origin.Web}, "p99 for the shop", []string{"prom"}},
		{"Orders", otherUser, origin.Origin{Kind: origin.API}, "", []string{"ch"}},
		{"Fleet", serviceKey, origin.Origin{Kind: origin.MCP, Client: "claude-code"}, "Checkout hosts", []string{"prom", "ch"}},
		{"Nodes", serviceKey, origin.Origin{Kind: origin.MCP}, "", []string{"prom"}},
		{"Queues", testUser, origin.Origin{Kind: origin.MCP, Client: "cursor"}, "", []string{"prom"}},
	}
	ids := map[string]string{}
	for _, s := range seeds {
		dash, _, err := r.service.Create(origin.With(t.Context(), s.from), s.by, specWithDatasources(s.title, s.desc, s.uids...), "create")
		if err != nil {
			t.Fatalf("create %s: %v", s.title, err)
		}
		ids[s.title] = dash.ID
	}
	return ids
}

func titlesOf(items []ListItem) []string {
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = item.Title
	}
	slices.Sort(titles)
	return titles
}

func TestListTotalCountMatchesTheFilteredRows(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	tests := []struct {
		name       string
		madeBy     string
		datasource string
		query      string
		want       []string
	}{
		{"no filter", "", "", "", []string{"Checkout Latency", "Fleet", "Nodes", "Orders", "Queues"}},
		{"me", "me", "", "", []string{"Checkout Latency", "Queues"}},
		{"user", "user:" + otherUserID, "", "", []string{"Orders"}},
		{"service account", "service_account:" + serviceKeyID, "", "", []string{"Fleet", "Nodes"}},
		{"any MCP client", "mcp", "", "", []string{"Fleet", "Nodes", "Queues"}},
		{"one MCP client", "mcp:claude-code", "", "", []string{"Fleet"}},
		{"unknown MCP client", "mcp:nobody", "", "", []string{}},
		{"agent", "agent", "", "", []string{}},
		{"datasource ch", "", "ch", "", []string{"Fleet", "Orders"}},
		{"datasource prom", "", "prom", "", []string{"Checkout Latency", "Fleet", "Nodes", "Queues"}},
		{"unused datasource", "", "nope", "", []string{}},
		{"q in title and description, any case", "", "", "CHECKOUT", []string{"Checkout Latency", "Fleet"}},
		{"q with a wildcard character", "", "", "%", []string{}},
		{"q matching nothing", "", "", "zzzz", []string{}},
		{"me and q", "me", "", "checkout", []string{"Checkout Latency"}},
		{"mcp and datasource", "mcp", "ch", "", []string{"Fleet"}},
		{"service account and q", "service_account:" + serviceKeyID, "", "hosts", []string{"Fleet"}},
		{"all three", "mcp:claude-code", "prom", "fleet", []string{"Fleet"}},
		{"all three without a match", "me", "ch", "checkout", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := ListParams{Limit: 50, Datasource: tt.datasource, Query: tt.query}
			if tt.madeBy != "" {
				parsed, err := ParseMadeBy(tt.madeBy, testUser)
				if err != nil {
					t.Fatal(err)
				}
				params.MadeBy = parsed
			}
			page, err := r.service.List(t.Context(), params)
			if err != nil {
				t.Fatal(err)
			}
			if got := titlesOf(page.Items); !slices.Equal(got, tt.want) {
				t.Fatalf("items = %v, want %v", got, tt.want)
			}
			if page.Total != len(tt.want) {
				t.Errorf("total_count = %d, want %d", page.Total, len(tt.want))
			}

			params.Limit = 1
			paged, err := r.service.List(t.Context(), params)
			if err != nil {
				t.Fatal(err)
			}
			if paged.Total != len(tt.want) {
				t.Errorf("total_count on a page of one = %d, want %d", paged.Total, len(tt.want))
			}
			if len(paged.Items) > 1 || (len(tt.want) > 1) != (paged.NextCursor != "") {
				t.Errorf("page of one has %d items and cursor %q for %d matches", len(paged.Items), paged.NextCursor, len(tt.want))
			}

			params.SkipTotal = true
			skipped, err := r.service.List(t.Context(), params)
			if err != nil {
				t.Fatal(err)
			}
			if skipped.Total != 0 || len(skipped.Items) != len(paged.Items) {
				t.Errorf("SkipTotal gave total %d and %d items, want 0 and %d", skipped.Total, len(skipped.Items), len(paged.Items))
			}
		})
	}
}

func TestListTotalCountIgnoresTheCursor(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	first, err := r.service.List(t.Context(), ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.service.List(t.Context(), ListParams{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 5 || second.Total != 5 || len(second.Items) != 2 {
		t.Errorf("totals %d and %d with %d items on page two, want 5, 5 and 2", first.Total, second.Total, len(second.Items))
	}
}

func TestListReportsDatasourcesAndCreator(t *testing.T) {
	r := newRig(t, false)
	ids := r.seedList(t)
	page, err := r.service.List(t.Context(), ListParams{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		switch item.ID {
		case ids["Fleet"]:
			if !slices.Equal(item.DatasourceUIDs, []string{"ch", "prom"}) {
				t.Errorf("Fleet datasources = %v", item.DatasourceUIDs)
			}
			if item.Creator.Origin != "mcp" || item.Creator.OriginClient != "claude-code" || item.Creator.Via != "direct" || item.Creator.Author.ID != serviceKeyID {
				t.Errorf("Fleet creator = %+v", item.Creator)
			}
			if item.Description != "Checkout hosts" {
				t.Errorf("Fleet description = %q", item.Description)
			}
		case ids["Orders"]:
			if item.Creator.Author.Kind != "user" || item.Creator.Author.ID != otherUserID || item.Creator.Origin != "api" {
				t.Errorf("Orders creator = %+v", item.Creator)
			}
		}
	}
}

func TestListCountsOnlyOpenRootThreads(t *testing.T) {
	r := newRig(t, false)
	busy := r.createDashboard(t, "rps")
	quiet := r.createDashboard(t, "rps")
	panel := "rps"

	open := r.createComment(t, busy, NewComment{Version: 1, PanelID: &panel, Body: "open root"})
	r.createComment(t, busy, NewComment{ParentID: mustParse(t, open.ID), Body: "reply"})
	r.createComment(t, busy, NewComment{ParentID: mustParse(t, open.ID), Body: "second reply"})
	resolved := r.createComment(t, busy, NewComment{Version: 1, PanelID: &panel, Body: "resolved root"})
	r.createComment(t, busy, NewComment{ParentID: mustParse(t, resolved.ID), Body: "reply to the resolved root"})
	if _, _, err := r.comments.SetStatus(t.Context(), testUser, busy, mustParse(t, resolved.ID), CommentResolved, nil); err != nil {
		t.Fatal(err)
	}

	page, err := r.service.List(t.Context(), ListParams{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, item := range page.Items {
		counts[item.ID] = item.OpenComments
	}
	if counts[busy.String()] != 1 || counts[quiet.String()] != 0 {
		t.Errorf("open comment counts = %v, want 1 for the busy dashboard and 0 for the quiet one", counts)
	}

	if _, _, err := r.comments.SetStatus(t.Context(), testUser, busy, mustParse(t, open.ID), CommentResolved, nil); err != nil {
		t.Fatal(err)
	}
	page, err = r.service.List(t.Context(), ListParams{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.OpenComments != 0 {
			t.Errorf("%s still counts %d open threads after every thread is resolved", item.ID, item.OpenComments)
		}
	}
}

func TestParseMadeBy(t *testing.T) {
	valid := map[string]MadeBy{
		"me":                                   {CreatorKind: "user", CreatorID: testUserID},
		"agent":                                {AgentOnly: true},
		"mcp":                                  {MCPOnly: true},
		"mcp:claude-code":                      {MCPOnly: true, MCPClient: "claude-code"},
		"user:" + otherUserID:                  {CreatorKind: "user", CreatorID: otherUserID},
		"service_account:" + serviceKeyID:      {CreatorKind: "service_account", CreatorID: serviceKeyID},
		"user:" + strings.ToUpper(otherUserID): {CreatorKind: "user", CreatorID: otherUserID},
	}
	for raw, want := range valid {
		got, err := ParseMadeBy(raw, testUser)
		if err != nil || got != want {
			t.Errorf("ParseMadeBy(%q) = %+v, %v, want %+v", raw, got, err, want)
		}
	}
	invalid := []string{
		"", "bogus", "mcp:", "mcp:a\x01b", "mcp:a\nb", "mcp:a\x7fb", "mcp:" + strings.Repeat("a", 65),
		"user:", "user:x", "user:" + otherUserID + "0", "service_account:not-a-uuid", "service_account:",
		"robot:" + otherUserID, "me:" + otherUserID, "agent:x", ":" + otherUserID,
	}
	for _, raw := range invalid {
		var target *InvalidInputError
		if _, err := ParseMadeBy(raw, testUser); !errors.As(err, &target) {
			t.Errorf("ParseMadeBy(%q) error = %v, want InvalidInputError", raw, err)
		}
	}
}

func TestMadeByMeUsesTheCallersKind(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	for caller, want := range map[auth.Principal][]string{
		serviceKey: {"Fleet", "Nodes"},
		otherUser:  {"Orders"},
	} {
		parsed, err := ParseMadeBy("me", caller)
		if err != nil {
			t.Fatal(err)
		}
		page, err := r.service.List(t.Context(), ListParams{Limit: 50, MadeBy: parsed})
		if err != nil {
			t.Fatal(err)
		}
		if got := titlesOf(page.Items); !slices.Equal(got, want) {
			t.Errorf("made_by=me for %s %s = %v, want %v", caller.Kind, caller.ID, got, want)
		}
	}
}

func TestDatasourceUsageCountsOnlyTheLatestVersion(t *testing.T) {
	r := newRig(t, false)
	dash, _, err := r.service.Create(t.Context(), testUser, specWithDatasources("Usage", "", "prom"), "create")
	if err != nil {
		t.Fatal(err)
	}
	check := func(label string, want map[string]int) {
		t.Helper()
		usage, err := r.service.DatasourceUsage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, uid := range []string{"prom", "ch"} {
			if usage[uid] != want[uid] {
				t.Errorf("%s: usage[%s] = %d, want %d", label, uid, usage[uid], want[uid])
			}
			page, err := r.service.List(t.Context(), ListParams{Limit: 1, Datasource: uid})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != usage[uid] {
				t.Errorf("%s: list total for %s = %d, usage = %d", label, uid, page.Total, usage[uid])
			}
		}
	}
	check("first version", map[string]int{"prom": 1})

	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, mustParse(t, dash.ID), 1, specWithDatasources("Usage", "", "ch"), "switch"); err != nil {
		t.Fatal(err)
	}
	check("second version", map[string]int{"ch": 1})
}
