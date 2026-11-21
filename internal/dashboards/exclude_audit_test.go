package dashboards

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestListLeavesOutTheAuditFolderFromRowsTotalAndPagingInEveryOrder(t *testing.T) {
	r := newRig(t, false)
	ids := r.seedList(t)
	trail := r.newFolder(t, "Trail")
	for _, title := range []string{"Sign-ins", "Reads"} {
		dash, _, err := r.service.Create(reading(t.Context()), testUser, trail, specWithDatasources(title, "", "prom"), "create")
		if err != nil {
			t.Fatal(err)
		}
		r.recordCheck(t, dash.ID, "no_data")
	}
	r.recordCheck(t, ids["Orders"], "no_data")

	everything := []string{"Checkout Latency", "Fleet", "Nodes", "Orders", "Queues", "Reads", "Sign-ins"}
	outsideTheAudit := []string{"Checkout Latency", "Fleet", "Nodes", "Orders", "Queues"}
	noDataOutside := []string{"Orders"}
	noDataEverywhere := []string{"Orders", "Reads", "Sign-ins"}

	tests := []struct {
		name         string
		recorded     bool
		excludeAudit bool
		problem      string
		want         []string
	}{
		{"not recorded, default", false, false, "", everything},
		{"not recorded, excluded", false, true, "", everything},
		{"recorded, default lists it", true, false, "", everything},
		{"recorded, excluded", true, true, "", outsideTheAudit},
		{"recorded, default with problem", true, false, "no_data", noDataEverywhere},
		{"recorded, excluded with problem", true, true, "no_data", noDataOutside},
	}
	for _, tt := range tests {
		for _, order := range []string{"updated", "folder", "views", "name"} {
			t.Run(fmt.Sprintf("%s by %s", tt.name, order), func(t *testing.T) {
				r.db.Exec(t, `DELETE FROM starter_content WHERE key = $1`, folders.AuditStarterKey)
				if tt.recorded {
					r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)
				}
				params := ListParams{Limit: 50, Problem: tt.problem, ExcludeAudit: tt.excludeAudit, ByFolder: order == "folder", ByViews: order == "views", ByName: order == "name"}
				page, err := r.service.List(reading(t.Context()), testUser, params)
				if err != nil {
					t.Fatal(err)
				}
				if got := titlesOf(page.Items); !slices.Equal(got, tt.want) || page.Total != len(tt.want) {
					t.Errorf("items %v with total %d, want %v with total %d", got, page.Total, tt.want, len(tt.want))
				}
			})
		}
	}
}

func TestListWithTheAuditFolderLeftOutPagesWithoutHolesAndMatchesItsTotal(t *testing.T) {
	r := newRig(t, false)
	r.seedList(t)
	trail := r.newFolder(t, "Trail")
	for _, title := range []string{"Sign-ins", "Reads", "Denials"} {
		if _, _, err := r.service.Create(reading(t.Context()), testUser, trail, specWithDatasources(title, "", "prom"), "create"); err != nil {
			t.Fatal(err)
		}
	}
	r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	var walked []string
	for offset := 0; ; offset += 2 {
		page, err := r.service.List(reading(t.Context()), testUser, ListParams{Limit: 2, Offset: offset, ExcludeAudit: true, ByFolder: true})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Fatalf("offset %d: total %d, want 5", offset, page.Total)
		}
		if len(page.Items) == 0 {
			break
		}
		walked = append(walked, titlesOf(page.Items)...)
	}
	slices.Sort(walked)
	if want := []string{"Checkout Latency", "Fleet", "Nodes", "Orders", "Queues"}; !slices.Equal(walked, want) {
		t.Errorf("walked %v, want %v", walked, want)
	}
}

func TestListRouteTakesExcludeAuditAsABoolean(t *testing.T) {
	b := seedBoards(t)
	trail := foldertest.Open(t, b.rig.db, "Trail")
	foldertest.Move(t, b.rig.db, b.open, trail)
	b.rig.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	for _, tt := range []struct {
		query string
		code  int
		total string
	}{
		{"", http.StatusOK, `"total_count":1`},
		{"?exclude_audit=false", http.StatusOK, `"total_count":1`},
		{"?exclude_audit=true", http.StatusOK, `"total_count":0`},
		{"?exclude_audit=yes", http.StatusBadRequest, ""},
	} {
		got := b.serve(testUser, http.MethodGet, "/dashboards"+tt.query, "")
		if got.Code != tt.code || !strings.Contains(got.Body.String(), tt.total) {
			t.Errorf("%q: %d %s, want %d containing %s", tt.query, got.Code, got.Body, tt.code, tt.total)
		}
	}
}

func TestExcludeAuditChangesNothingForSomeoneWhoCannotSeeTheAuditFolder(t *testing.T) {
	b := seedBoards(t)
	trail := foldertest.AdminsOnly(t, b.rig.db, "Trail")
	if _, _, err := b.rig.service.Create(reading(t.Context()), adminUser, trail, specWithDatasources("Sign-ins", "", "prom"), "create"); err != nil {
		t.Fatal(err)
	}
	b.rig.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	for _, order := range []string{"updated", "folder", "views", "name"} {
		var seen [2]ListPage
		for i, exclude := range []bool{false, true} {
			page, err := b.rig.service.List(reading(t.Context()), otherUser, ListParams{Limit: 50, ExcludeAudit: exclude, ByFolder: order == "folder", ByViews: order == "views", ByName: order == "name"})
			if err != nil {
				t.Fatal(err)
			}
			seen[i] = page
		}
		if got, want := titlesOf(seen[1].Items), titlesOf(seen[0].Items); !slices.Equal(got, want) || seen[1].Total != seen[0].Total || seen[0].Total != 1 {
			t.Errorf("by %s: with exclude_audit %v (total %d), without %v (total %d), want the same single dashboard", order, got, seen[1].Total, want, seen[0].Total)
		}
	}
}

func TestTagsLeaveOutTheAuditFoldersWhenAsked(t *testing.T) {
	b := seedBoards(t)
	trail := foldertest.AdminsOnly(t, b.rig.db, "Trail")
	if _, _, err := b.rig.service.Create(reading(t.Context()), adminUser, trail, boardSpec("Sign-ins", "", "prom", "audit"), "create"); err != nil {
		t.Fatal(err)
	}
	b.rig.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, folders.AuditStarterKey, trail)

	tagsOf := func(exclude bool) map[string]int {
		tags, err := b.rig.service.Tags(reading(t.Context()), adminUser, exclude)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]int{}
		for _, tag := range tags {
			out[tag.Tag] = tag.Dashboards
		}
		return out
	}
	if got := tagsOf(false); got["audit"] != 1 {
		t.Errorf("tags without exclude_audit = %v, want audit counted once", got)
	}
	if got := tagsOf(true); got["audit"] != 0 || got["open-tag"] != 1 {
		t.Errorf("tags with exclude_audit = %v, want no audit tag and the others kept", got)
	}
	rec := b.serve(adminUser, http.MethodGet, "/dashboards/tags?exclude_audit=true", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"audit"`) {
		t.Errorf("GET /dashboards/tags?exclude_audit=true = %d %s", rec.Code, rec.Body)
	}
	if rec := b.serve(adminUser, http.MethodGet, "/dashboards/tags?exclude_audit=maybe", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("exclude_audit=maybe = %d, want 400", rec.Code)
	}
}
