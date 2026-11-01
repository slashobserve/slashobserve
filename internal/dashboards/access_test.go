package dashboards

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

const (
	privateGroup  = "payroll-team"
	hiddenTitle   = "Payroll board"
	hiddenTag     = "payroll-tag"
	hiddenWords   = "salary figures"
	openTitle     = "Open board"
	sharedTag     = "shared-tag"
	roleErrorCode = "forbidden_role"
)

var (
	viewerNoGrant    = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Vic", Email: "vic@example.com", Role: auth.RoleViewer}
	commenterNoGrant = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Cam", Email: "cam@example.com", Role: auth.RoleCommenter}
	groupViewer      = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Gia", Email: "gia@example.com", Role: auth.RoleViewer, Groups: []string{privateGroup}}
	adminUser        = auth.Principal{Kind: auth.KindUser, ID: "77777777-7777-4777-8777-777777777777", Name: "Root", Email: "root@example.com", Role: auth.RoleAdmin}
)

func boardSpec(title, description, datasource string, tags ...string) []byte {
	encoded, err := json.Marshal(append([]string{}, tags...))
	if err != nil {
		panic(err)
	}
	return fmt.Appendf(nil, `{"specVersion":"0","title":%q,"description":%q,"tags":%s,"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":%q,"queries":[{"query":"up"}]}]}`, title, description, encoded, datasource)
}

type boards struct {
	rig    *rig
	open   pgtype.UUID
	hidden pgtype.UUID
}

func seedBoards(t *testing.T) boards {
	t.Helper()
	r := newRig(t, false)
	create := func(spec []byte) pgtype.UUID {
		dash, _, err := r.service.Create(t.Context(), testUser, pgtype.UUID{}, spec, "create")
		if err != nil {
			t.Fatal(err)
		}
		id, err := db.ParseUUID(dash.ID)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	b := boards{rig: r}
	b.open = create(boardSpec(openTitle, "visible words", "prom", "open-tag", sharedTag))
	b.hidden = create(boardSpec(hiddenTitle, hiddenWords, "ch", hiddenTag, sharedTag))
	foldertest.Move(t, r.db, b.hidden, foldertest.Restricted(t, r.db, "Payroll", privateGroup))
	return b
}

func (b boards) serve(by auth.Principal, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	b.rig.service.Register(auth.NewRoutes(router.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})))
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestOffsetNeverCountsHiddenDashboards(t *testing.T) {
	b := seedBoards(t)
	second := func(by auth.Principal, offset int) ListPage {
		t.Helper()
		page, err := b.rig.service.List(t.Context(), by, ListParams{ByFolder: true, Limit: 1, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	blind := second(otherUser, 1)
	if len(blind.Items) != 0 || blind.Total != 1 {
		t.Errorf("caller without access: %d items and total %d, want 0 and 1", len(blind.Items), blind.Total)
	}

	first, next := second(adminUser, 0), second(adminUser, 1)
	if len(first.Items) != 1 || len(next.Items) != 1 || first.Items[0].ID == next.Items[0].ID {
		t.Errorf("admin pages repeat or miss a dashboard: %v then %v", itemIDs(first.Items), itemIDs(next.Items))
	}
	if first.Total != 2 || next.Total != 2 {
		t.Errorf("admin totals = %d and %d, want 2", first.Total, next.Total)
	}
}

func TestRestrictedFolderDashboardsNeverLeakIntoListings(t *testing.T) {
	b := seedBoards(t)
	callers := []struct {
		name string
		by   auth.Principal
		sees bool
	}{
		{"Editor with no grant", otherUser, false},
		{"Commenter with no grant", commenterNoGrant, false},
		{"Viewer with no grant", viewerNoGrant, false},
		{"service account key that did not create the folder", serviceKey, false},
		{"Viewer in the granted group", groupViewer, true},
		{"Admin", adminUser, true},
	}
	searches := []struct {
		name        string
		params      ListParams
		matchOpen   bool
		matchHidden bool
	}{
		{"everything", ListParams{}, true, true},
		{"grouped by folder", ListParams{ByFolder: true}, true, true},
		{"q on the hidden title", ListParams{Query: "PAYROLL"}, false, true},
		{"q on the hidden description", ListParams{Query: "salary"}, false, true},
		{"q on a word in both titles", ListParams{Query: "board"}, true, true},
		{"the hidden tag", ListParams{Tags: []string{hiddenTag}}, false, true},
		{"the shared tag", ListParams{Tags: []string{sharedTag}}, true, true},
		{"the hidden dashboard's datasource", ListParams{Datasource: "ch"}, false, true},
		{"the open dashboard's datasource", ListParams{Datasource: "prom"}, true, false},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			for _, s := range searches {
				params := s.params
				params.Limit = 50
				page, err := b.rig.service.List(t.Context(), c.by, params)
				if err != nil {
					t.Fatalf("%s: %v", s.name, err)
				}
				var want []string
				if s.matchOpen {
					want = append(want, openTitle)
				}
				if s.matchHidden && c.sees {
					want = append(want, hiddenTitle)
				}
				slices.Sort(want)
				got := titlesOf(page.Items)
				if !slices.Equal(got, want) || page.Total != len(want) {
					t.Errorf("%s: items %v and total %d, want %v and %d", s.name, got, page.Total, want, len(want))
				}
			}

			tags, err := b.rig.service.Tags(t.Context(), c.by)
			if err != nil {
				t.Fatal(err)
			}
			wantTags := map[string]int{"open-tag": 1, sharedTag: 1}
			wantUsage := map[string]int{"prom": 1}
			if c.sees {
				wantTags = map[string]int{"open-tag": 1, sharedTag: 2, hiddenTag: 1}
				wantUsage = map[string]int{"prom": 1, "ch": 1}
			}
			gotTags := map[string]int{}
			for _, tag := range tags {
				gotTags[tag.Tag] = tag.Dashboards
			}
			if fmt.Sprint(gotTags) != fmt.Sprint(wantTags) {
				t.Errorf("tags = %v, want %v", gotTags, wantTags)
			}

			usage, err := b.rig.service.DatasourceUsage(t.Context(), c.by)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(usage) != fmt.Sprint(wantUsage) {
				t.Errorf("datasource usage = %v, want %v", usage, wantUsage)
			}
		})
	}
}

func TestRestrictedFolderDashboardsNeverLeakThroughTheRoutes(t *testing.T) {
	b := seedBoards(t)
	hiddenID := b.hidden.String()
	leaks := []string{hiddenTitle, hiddenTag, hiddenWords, hiddenID}

	for _, by := range []auth.Principal{otherUser, commenterNoGrant, viewerNoGrant, serviceKey} {
		t.Run(by.Name, func(t *testing.T) {
			for _, path := range []string{"/dashboards", "/dashboards?order=folder", "/dashboards?q=payroll", "/dashboards?q=salary", "/dashboards?tag=" + hiddenTag, "/dashboards/tags"} {
				rec := b.serve(by, http.MethodGet, path, "")
				if rec.Code != http.StatusOK {
					t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body)
				}
				for _, leak := range leaks {
					if strings.Contains(rec.Body.String(), leak) {
						t.Errorf("GET %s leaks %q: %s", path, leak, rec.Body)
					}
				}
			}
			for _, path := range []string{"", "/versions", "/versions/1", "/diff"} {
				rec := b.serve(by, http.MethodGet, "/dashboards/"+hiddenID+path, "")
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
					t.Errorf("GET %s = %d %s, want 403 %s", path, rec.Code, rec.Body, roleErrorCode)
				}
				for _, leak := range leaks[:3] {
					if strings.Contains(rec.Body.String(), leak) {
						t.Errorf("GET %s leaks %q: %s", path, leak, rec.Body)
					}
				}
			}
		})
	}

	t.Run("Viewer in the granted group", func(t *testing.T) {
		rec := b.serve(groupViewer, http.MethodGet, "/dashboards?q=payroll", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), hiddenTitle) {
			t.Fatalf("search = %d %s, want the hidden dashboard", rec.Code, rec.Body)
		}
		rec = b.serve(groupViewer, http.MethodGet, "/dashboards/tags", "")
		if !strings.Contains(rec.Body.String(), hiddenTag) {
			t.Fatalf("tags = %d %s, want %s", rec.Code, rec.Body, hiddenTag)
		}
		rec = b.serve(groupViewer, http.MethodGet, "/dashboards/"+hiddenID, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), hiddenTitle) {
			t.Fatalf("get = %d %s", rec.Code, rec.Body)
		}
	})
}

func TestHiddenDashboardsRefuseWritesFromCallersWithoutAccess(t *testing.T) {
	b := seedBoards(t)
	hiddenID := b.hidden.String()
	for _, by := range []auth.Principal{otherUser, serviceKey} {
		for _, tc := range []struct {
			method, path, body string
		}{
			{http.MethodPost, "/versions", `{"spec":` + string(boardSpec("Edited", "", "ch")) + `,"summary":"edit","parent_version":1}`},
			{http.MethodPatch, "", `{"folder_id":"` + b.open.String() + `"}`},
			{http.MethodDelete, "", ""},
			{http.MethodPost, "/duplicate", `{"title":"Copy"}`},
		} {
			rec := b.serve(by, tc.method, "/dashboards/"+hiddenID+tc.path, tc.body)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
				t.Errorf("%s %s as %s = %d %s, want 403 %s", tc.method, tc.path, by.Kind, rec.Code, rec.Body, roleErrorCode)
			}
		}
	}
	page, err := b.rig.service.List(t.Context(), adminUser, ListParams{Limit: 50})
	if err != nil || page.Total != 2 {
		t.Fatalf("after the refused writes: total %d, err %v", page.Total, err)
	}
	if _, ver, err := b.rig.service.Get(t.Context(), b.hidden); err != nil || ver.Number != 1 || ver.Spec.Title != hiddenTitle {
		t.Fatalf("hidden dashboard changed: version %d, err %v", ver.Number, err)
	}
}

func TestViewersAndCommentersWithoutAGrantCannotEditMoveOrDeleteDashboards(t *testing.T) {
	for _, by := range []auth.Principal{viewerNoGrant, commenterNoGrant} {
		t.Run(string(by.Role), func(t *testing.T) {
			b := seedBoards(t)
			id := b.open.String()
			for _, tc := range []struct {
				name, method, path, body string
			}{
				{"create", http.MethodPost, "/dashboards", `{"spec":` + string(boardSpec("New", "", "prom")) + `}`},
				{"edit", http.MethodPost, "/dashboards/" + id + "/versions", `{"spec":` + string(boardSpec("Edited", "", "prom")) + `,"summary":"edit","parent_version":1}`},
				{"restore", http.MethodPost, "/dashboards/" + id + "/versions/1/restore", `{"parent_version":1}`},
				{"move", http.MethodPatch, "/dashboards/" + id, `{"folder_id":"` + b.open.String() + `"}`},
				{"delete", http.MethodDelete, "/dashboards/" + id, ""},
				{"duplicate", http.MethodPost, "/dashboards/" + id + "/duplicate", `{"title":"Copy"}`},
			} {
				rec := b.serve(by, tc.method, tc.path, tc.body)
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
					t.Errorf("%s = %d %s, want 403 %s", tc.name, rec.Code, rec.Body, roleErrorCode)
				}
			}
			if rec := b.serve(by, http.MethodGet, "/dashboards/"+id, ""); rec.Code != http.StatusOK {
				t.Errorf("read = %d %s, want 200", rec.Code, rec.Body)
			}
			if _, ver, err := b.rig.service.Get(t.Context(), b.open); err != nil || ver.Number != 1 {
				t.Errorf("open dashboard changed: version %d, err %v", ver.Number, err)
			}
		})
	}

	t.Run("an Editor does edit, move and delete", func(t *testing.T) {
		b := seedBoards(t)
		id := b.open.String()
		for _, tc := range []struct {
			name, method, path, body string
			want                     int
		}{
			{"edit", http.MethodPost, "/dashboards/" + id + "/versions", `{"spec":` + string(boardSpec("Edited", "", "prom")) + `,"summary":"edit","parent_version":1}`, http.StatusCreated},
			{"move", http.MethodPatch, "/dashboards/" + id, `{"folder_id":"` + openFolder(t, b) + `"}`, http.StatusOK},
			{"delete", http.MethodDelete, "/dashboards/" + id, "", http.StatusNoContent},
		} {
			if rec := b.serve(otherUser, tc.method, tc.path, tc.body); rec.Code != tc.want {
				t.Errorf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
			}
		}
	})
}

func openFolder(t *testing.T, b boards) string {
	t.Helper()
	folder, err := b.rig.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	return folder.ID.String()
}
