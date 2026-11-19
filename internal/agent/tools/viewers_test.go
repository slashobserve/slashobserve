package tools

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/viewers"
)

func TestListDashboardViewersNamesTheOtherPeopleLookingAndHonoursFolderAccess(t *testing.T) {
	r := newAccessRig(t)
	boards := r.seedHidden(t)
	ada := r.person(t, "ada", auth.RoleViewer)
	bo := r.person(t, "bo", auth.RoleViewer)
	service := viewers.New(r.db.DB)
	for _, p := range []auth.Principal{ada, bo} {
		if err := service.Record(t.Context(), p, boards.open); err != nil {
			t.Fatal(err)
		}
	}

	res := r.call(t, ada, nameListDashboardViewers, `{"dashboard_id":"`+boards.open.String()+`"}`)
	out, ok := res.Content.(viewerListOut)
	if !ok || len(out.Viewers) != 1 || out.Viewers[0].Name != "bo" || out.Viewers[0].ID != bo.ID {
		t.Fatalf("what Ada sees = %s", textOf(t, res))
	}
	if res := r.call(t, ada, nameListDashboardViewers, `{"dashboard_id":"`+boards.hidden.String()+`"}`); !res.IsError || errorCode(t, res) != codeNotFound {
		t.Fatalf("a dashboard in a folder Ada cannot see = %s", textOf(t, res))
	}
	if res := r.call(t, ada, nameListDashboardViewers, `{"dashboard_id":"nope"}`); !res.IsError || errorCode(t, res) != codeInvalidRequest {
		t.Fatalf("a bad id = %s", textOf(t, res))
	}
}
