package dashboards

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	otherUserID  = "22222222-2222-4222-8222-222222222222"
	serviceKeyID = "33333333-3333-4333-8333-333333333333"
)

var (
	otherUser  = auth.Principal{Kind: auth.KindUser, ID: otherUserID, Name: "Bob", Email: "bob@example.com", Role: auth.RoleEditor}
	serviceKey = auth.Principal{Kind: auth.KindServiceAccount, ID: serviceKeyID, Name: "ci", Role: auth.RoleEditor}
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
		dash, _, err := r.service.Create(origin.With(reading(t.Context()), s.from), s.by, pgtype.UUID{}, specWithDatasources(s.title, s.desc, s.uids...), "create")
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
			page, err := r.service.List(reading(t.Context()), testUser, params)
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
			paged, err := r.service.List(reading(t.Context()), testUser, params)
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
			skipped, err := r.service.List(reading(t.Context()), testUser, params)
			if err != nil {
				t.Fatal(err)
			}
			if skipped.Total != 0 || len(skipped.Items) != len(paged.Items) {
				t.Errorf("SkipTotal gave total %d and %d items, want 0 and %d", skipped.Total, len(skipped.Items), len(paged.Items))
			}
		})
	}
}

func taggedSpec(title string, tags ...string) []byte {
	encoded, err := json.Marshal(append([]string{}, tags...))
	if err != nil {
		panic(err)
	}
	return fmt.Appendf(nil, `{"specVersion":"0","title":%q,"tags":%s,"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}]}`, title, encoded)
}

func TestListFiltersByEveryGivenTag(t *testing.T) {
	r := newRig(t, false)
	for title, tags := range map[string][]string{
		"Shop":     {"demo", "shop"},
		"Fleet":    {"demo"},
		"Untagged": nil,
	} {
		if _, _, err := r.service.Create(reading(t.Context()), testUser, pgtype.UUID{}, taggedSpec(title, tags...), "create"); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{"no tag", nil, []string{"Fleet", "Shop", "Untagged"}},
		{"one tag", []string{"demo"}, []string{"Fleet", "Shop"}},
		{"two tags", []string{"demo", "shop"}, []string{"Shop"}},
		{"a tag nobody has", []string{"nope"}, []string{}},
		{"one tag and one nobody has", []string{"demo", "nope"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, Tags: tt.tags})
			if err != nil {
				t.Fatal(err)
			}
			if got := titlesOf(page.Items); !slices.Equal(got, tt.want) {
				t.Fatalf("items = %v, want %v", got, tt.want)
			}
			if page.Total != len(tt.want) {
				t.Errorf("total_count = %d, want %d", page.Total, len(tt.want))
			}
		})
	}
}

func TestListTotalCountIgnoresTheCursor(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	first, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 5 || second.Total != 5 || len(second.Items) != 2 {
		t.Errorf("totals %d and %d with %d items on page two, want 5, 5 and 2", first.Total, second.Total, len(second.Items))
	}
}

func itemIDs(items []ListItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func TestListOffsetWalksTheSameOrder(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	for _, byFolder := range []bool{false, true} {
		t.Run(fmt.Sprintf("by folder %v", byFolder), func(t *testing.T) {
			whole, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 5, ByFolder: byFolder})
			if err != nil {
				t.Fatal(err)
			}
			var walked []string
			for _, offset := range []int{0, 2, 4} {
				page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Offset: offset, ByFolder: byFolder})
				if err != nil {
					t.Fatal(err)
				}
				if page.Total != 5 {
					t.Errorf("offset %d: total = %d, want 5", offset, page.Total)
				}
				walked = append(walked, itemIDs(page.Items)...)
			}
			if want := itemIDs(whole.Items); !slices.Equal(walked, want) {
				t.Errorf("offset pages = %v, want %v", walked, want)
			}
			past, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Offset: 5, ByFolder: byFolder})
			if err != nil {
				t.Fatal(err)
			}
			if len(past.Items) != 0 || past.Total != 5 {
				t.Errorf("past the end: %d items and total %d, want 0 and 5", len(past.Items), past.Total)
			}
		})
	}
}

func TestListReportsDatasourcesAndCreator(t *testing.T) {
	r := newRig(t, false)
	ids := r.seedList(t)
	page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50})
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

func TestListCountsOnlyOpenThreads(t *testing.T) {
	r := newRig(t, false)
	busy := r.createDashboard(t, "rps")
	quiet := r.createDashboard(t, "rps")
	r.insertThread(t, busy, "open")
	r.insertThread(t, busy, "open")
	r.insertThread(t, busy, "resolved")

	count := func() map[string]int {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		for _, item := range page.Items {
			counts[item.ID] = item.OpenComments
		}
		return counts
	}
	if counts := count(); counts[busy.String()] != 2 || counts[quiet.String()] != 0 {
		t.Errorf("open comment counts = %v, want 2 for the busy dashboard and 0 for the quiet one", counts)
	}

	r.db.Exec(t, `UPDATE threads SET status = 'resolved', resolved_at = now(), resolved_by_kind = 'user', resolved_by_id = gen_random_uuid(), resolved_by_name = 'x' WHERE dashboard_id = $1 AND status = 'open'`, busy)
	if counts := count(); counts[busy.String()] != 0 {
		t.Errorf("the busy dashboard still counts %d open threads after every thread is resolved", counts[busy.String()])
	}
}

func (r *rig) insertThread(t *testing.T, dash pgtype.UUID, status string) {
	t.Helper()
	_, err := r.db.SQLDB(t).ExecContext(reading(t.Context()), `INSERT INTO threads (dashboard_id, dashboard_version, status, resolved_at, resolved_by_kind, resolved_by_id, resolved_by_name)
VALUES ($1, 1, $2, CASE WHEN $2 = 'resolved' THEN now() END, CASE WHEN $2 = 'resolved' THEN 'user' END, CASE WHEN $2 = 'resolved' THEN gen_random_uuid() END, CASE WHEN $2 = 'resolved' THEN 'x' END)`, dash, status)
	if err != nil {
		t.Fatal(err)
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
	for _, tt := range []struct {
		caller auth.Principal
		want   []string
	}{
		{serviceKey, []string{"Fleet", "Nodes"}},
		{otherUser, []string{"Orders"}},
	} {
		caller, want := tt.caller, tt.want
		parsed, err := ParseMadeBy("me", caller)
		if err != nil {
			t.Fatal(err)
		}
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, MadeBy: parsed})
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
	dash, _, err := r.service.Create(reading(t.Context()), testUser, pgtype.UUID{}, specWithDatasources("Usage", "", "prom"), "create")
	if err != nil {
		t.Fatal(err)
	}
	check := func(label string, want map[string]int) {
		t.Helper()
		usage, err := r.service.DatasourceUsage(reading(t.Context()), testUser)
		if err != nil {
			t.Fatal(err)
		}
		for _, uid := range []string{"prom", "ch"} {
			if usage[uid] != want[uid] {
				t.Errorf("%s: usage[%s] = %d, want %d", label, uid, usage[uid], want[uid])
			}
			page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 1, Datasource: uid})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != usage[uid] {
				t.Errorf("%s: list total for %s = %d, usage = %d", label, uid, page.Total, usage[uid])
			}
		}
	}
	check("first version", map[string]int{"prom": 1})

	if _, _, _, err := r.service.AddVersion(reading(t.Context()), testUser, mustParse(t, dash.ID), 1, specWithDatasources("Usage", "", "ch"), "switch"); err != nil {
		t.Fatal(err)
	}
	check("second version", map[string]int{"ch": 1})
}

func (r *rig) recordCheck(t *testing.T, id, kind string) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO dashboard_health (dashboard_id, version, checked_at, issues)
		VALUES ($1, 1, now(), jsonb_build_array(jsonb_build_object('kind', $2::text)))`, id, kind)
}

func TestListFiltersByHealthProblemInBothOrders(t *testing.T) {
	r := newRig(t, false)
	other, err := r.db.Queries().CreateFolder(reading(t.Context()), dbsqlc.CreateFolderParams{Title: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, _, err := r.service.Create(reading(t.Context()), testUser, other.ID, specWithDatasources("Elsewhere", "", "prom"), "create")
	if err != nil {
		t.Fatal(err)
	}
	ids := r.seedList(t)
	r.recordCheck(t, elsewhere.ID, "no_data")
	r.recordCheck(t, ids["Orders"], "no_data")
	r.recordCheck(t, ids["Fleet"], "query_failed")
	r.db.Exec(t, `UPDATE dashboards SET created_at = now() - interval '100 days' WHERE id = $1`, ids["Nodes"])

	tests := []struct {
		problem  string
		updated  []string
		byFolder []string
	}{
		{"no_data", []string{"Orders", "Elsewhere"}, []string{"Elsewhere", "Orders"}},
		{"query_failed", []string{"Fleet"}, []string{"Fleet"}},
		{"variable_failed", []string{}, []string{}},
		{"unopened", []string{"Nodes"}, []string{"Nodes"}},
		{"any", []string{"Nodes", "Fleet", "Orders", "Elsewhere"}, []string{"Elsewhere", "Nodes", "Fleet", "Orders"}},
		{"", []string{"Queues", "Nodes", "Fleet", "Orders", "Checkout Latency", "Elsewhere"}, []string{"Elsewhere", "Queues", "Nodes", "Fleet", "Orders", "Checkout Latency"}},
	}
	for _, tt := range tests {
		for _, byFolder := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q by folder %v", tt.problem, byFolder), func(t *testing.T) {
				want := tt.updated
				if byFolder {
					want = tt.byFolder
				}
				page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, Problem: tt.problem, ByFolder: byFolder})
				if err != nil {
					t.Fatal(err)
				}
				got := make([]string, len(page.Items))
				for i, item := range page.Items {
					got[i] = item.Title
				}
				if !slices.Equal(got, want) || page.Total != len(want) {
					t.Errorf("items %v with total %d, want %v with total %d", got, page.Total, want, len(want))
				}
			})
		}
	}
}

func TestListFiltersByProblemKeepsADashboardWaitingForARecheckOfAProblemOrARepair(t *testing.T) {
	r := newRig(t, false)
	create := func(title string) string {
		dash, _, err := r.service.Create(reading(t.Context()), testUser, pgtype.UUID{}, specWithDatasources(title, "", "prom"), "create")
		if err != nil {
			t.Fatal(err)
		}
		return dash.ID
	}
	edit := func(id, title string) {
		if _, _, _, err := r.service.AddVersion(reading(t.Context()), testUser, mustParse(t, id), 1, specWithDatasources(title, "edited", "prom"), "edit"); err != nil {
			t.Fatal(err)
		}
	}
	hadProblem, repaired, cleanOutdated, current := create("Had a problem"), create("Repaired"), create("Clean outdated"), create("Current")
	r.recordCheck(t, hadProblem, "query_failed")
	r.recordCheck(t, current, "no_data")
	for _, id := range []string{repaired, cleanOutdated} {
		r.db.Exec(t, `INSERT INTO dashboard_health (dashboard_id, version, checked_at, issues) VALUES ($1, 1, now(), '[]'::jsonb)`, id)
	}
	r.db.Exec(t, `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
		VALUES ($1, 'prompt', $2, 1, 'user', $3, 'Ada')`, repaired, repairprompt.Prefix, testUserID)
	edit(hadProblem, "Had a problem")
	edit(repaired, "Repaired")
	edit(cleanOutdated, "Clean outdated")

	for _, byFolder := range []bool{false, true} {
		for problem, want := range map[string][]string{
			"any":          {"Current", "Had a problem", "Repaired"},
			"query_failed": {},
			"no_data":      {"Current"},
		} {
			page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, Problem: problem, ByFolder: byFolder})
			if err != nil {
				t.Fatal(err)
			}
			got := titlesOf(page.Items)
			slices.Sort(got)
			if !slices.Equal(got, want) || page.Total != len(want) {
				t.Errorf("problem %q by folder %v: items %v with total %d, want %v with total %d", problem, byFolder, got, page.Total, want, len(want))
			}
		}
	}
}

func (r *rig) viewDays(t *testing.T, dashboardID string, subject string, daysAgo ...int) {
	t.Helper()
	user, err := r.db.Queries().UpsertUser(reading(t.Context()), dbsqlc.UpsertUserParams{OidcIssuer: "views", OidcSubject: subject, Email: subject + "@example.com", Name: subject})
	if err != nil {
		t.Fatal(err)
	}
	for _, ago := range daysAgo {
		r.db.Exec(t, `INSERT INTO dashboard_view_days (dashboard_id, user_id, day) VALUES ($1, $2, current_date - $3::int)`, dashboardID, user.ID.String(), ago)
	}
}

func TestListByViewsSortsByPersonDaysInTheWindow(t *testing.T) {
	r := newRig(t, false)
	ids := r.seedList(t)
	r.viewDays(t, ids["Orders"], "ann", 0, 1)
	r.viewDays(t, ids["Orders"], "bob", 0)
	r.viewDays(t, ids["Fleet"], "ann", 0, 2, 3)
	r.viewDays(t, ids["Queues"], "ann", 5)
	r.viewDays(t, ids["Nodes"], "ann", 30, 90)

	whole, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, ByViews: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	counts := map[string]int{}
	for _, item := range whole.Items {
		got = append(got, item.Title)
		counts[item.Title] = item.ViewCount
	}
	want := []string{"Fleet", "Orders", "Queues", "Nodes", "Checkout Latency"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v (equal counts go to the most recently updated)", got, want)
	}
	if counts["Fleet"] != 3 || counts["Orders"] != 3 || counts["Queues"] != 1 || counts["Nodes"] != 0 {
		t.Errorf("counts = %v, want Fleet 3, Orders 3, Queues 1 and Nodes 0 (30 and 90 days ago are outside the window)", counts)
	}

	var walked []string
	cursor := ""
	for range 5 {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Cursor: cursor, ByViews: true, SkipTotal: true})
		if err != nil {
			t.Fatal(err)
		}
		walked = append(walked, itemIDs(page.Items)...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if want := itemIDs(whole.Items); !slices.Equal(walked, want) {
		t.Errorf("cursor pages = %v, want %v", walked, want)
	}

	var offsets []string
	for _, offset := range []int{0, 2, 4} {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Offset: offset, ByViews: true})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Errorf("offset %d: total = %d, want 5", offset, page.Total)
		}
		offsets = append(offsets, itemIDs(page.Items)...)
	}
	if want := itemIDs(whole.Items); !slices.Equal(offsets, want) {
		t.Errorf("offset pages = %v, want %v", offsets, want)
	}
}

func TestListRejectsACursorFromAnotherOrder(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	recent, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	byViews, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, ByViews: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, ByViews: true, Cursor: recent.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a recency cursor under order views: %v, want ErrInvalidCursor", err)
	}
	if _, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Cursor: byViews.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a views cursor under order updated: %v, want ErrInvalidCursor", err)
	}
	if _, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, ByFolder: true, Cursor: byViews.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a views cursor under order folder: %v, want ErrInvalidCursor", err)
	}
	byName, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, ByName: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Cursor: byName.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a name cursor under order updated: %v, want ErrInvalidCursor", err)
	}
	if _, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, ByName: true, Cursor: byViews.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a views cursor under order name: %v, want ErrInvalidCursor", err)
	}
}

func TestListByNameSortsByLowerCaseTitleAcrossFolders(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)

	whole, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 50, ByName: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range whole.Items {
		got = append(got, item.Title)
	}
	want := slices.Clone(got)
	slices.SortStableFunc(want, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	if len(got) != 5 || !slices.Equal(got, want) {
		t.Fatalf("order = %v, want the five titles sorted ignoring case: %v", got, want)
	}

	var walked []string
	cursor := ""
	for range 5 {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Cursor: cursor, ByName: true, SkipTotal: true})
		if err != nil {
			t.Fatal(err)
		}
		walked = append(walked, itemIDs(page.Items)...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if want := itemIDs(whole.Items); !slices.Equal(walked, want) {
		t.Errorf("cursor pages = %v, want %v", walked, want)
	}

	var offsets []string
	for _, offset := range []int{0, 2, 4} {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Offset: offset, ByName: true})
		if err != nil {
			t.Fatal(err)
		}
		offsets = append(offsets, itemIDs(page.Items)...)
	}
	if want := itemIDs(whole.Items); !slices.Equal(offsets, want) {
		t.Errorf("offset pages = %v, want %v", offsets, want)
	}
}
