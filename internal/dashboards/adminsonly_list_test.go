package dashboards

import (
	"net/http"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnAdminsOnlyFolderKeepsItsDashboardsOffItsNonAdminCreatorsSidebar(t *testing.T) {
	b := seedBoards(t)
	foldertest.Move(t, b.rig.db, b.hidden, foldertest.AdminsOnlyMadeBy(t, b.rig.db, "Salaries", otherUser))

	for _, params := range []ListParams{{Limit: 50}, {Limit: 50, ByFolder: true}} {
		creator, err := b.rig.service.List(reading(t.Context()), otherUser, params)
		if err != nil {
			t.Fatal(err)
		}
		if got := titlesOf(creator.Items); !slices.Equal(got, []string{openTitle}) || creator.Total != 1 {
			t.Fatalf("by folder %v: the folder's creator lists %v (total %d), want only %q", params.ByFolder, got, creator.Total, openTitle)
		}
		admin, err := b.rig.service.List(reading(t.Context()), adminUser, params)
		if err != nil {
			t.Fatal(err)
		}
		if got := titlesOf(admin.Items); !slices.Contains(got, hiddenTitle) {
			t.Fatalf("by folder %v: an Admin lists %v, want %q", params.ByFolder, got, hiddenTitle)
		}
	}
	if got := b.serve(otherUser, http.MethodGet, "/dashboards/"+b.hidden.String(), ""); got.Code != http.StatusNotFound {
		t.Fatalf("the creator opens the dashboard: %d %s, want 404", got.Code, got.Body)
	}
}
