package tools

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/stars"
)

func (r *accessRig) pinning() *accessRig {
	access := folders.NewAccess(r.db.DB)
	withStars := *r
	withStars.registry = New(Deps{Dashboards: r.service, Folders: access, Stars: stars.New(r.db.DB, access), Logger: slog.New(slog.DiscardHandler)})
	return &withStars
}

func (r *accessRig) pinned(t *testing.T, by auth.Principal) []pinnedItemOut {
	t.Helper()
	res := r.call(t, by, nameListPinnedDashboards, `{}`)
	out, ok := res.Content.(pinnedListOut)
	if res.IsError || !ok {
		t.Fatalf("list_pinned_dashboards = %s", textOf(t, res))
	}
	return out.Dashboards
}

func TestPinAndUnpinDashboardsForAPerson(t *testing.T) {
	r := newAccessRig(t).pinning()
	eve := r.person(t, "eve", auth.RoleEditor)
	bob := r.person(t, "bob", auth.RoleEditor)
	first, second := r.seed(t, "First"), r.seed(t, "Second")

	pin := func(by auth.Principal, tool string, id string) pinnedOut {
		t.Helper()
		res := r.call(t, by, tool, fmt.Sprintf(`{"dashboard_id":%q}`, id))
		out, ok := res.Content.(pinnedOut)
		if res.IsError || !ok || out.DashboardID != id {
			t.Fatalf("%s = %s", tool, textOf(t, res))
		}
		return out
	}
	if !pin(eve, namePinDashboard, first.String()).Pinned {
		t.Fatal("pin_dashboard answered pinned false")
	}
	pin(eve, namePinDashboard, first.String())
	pin(eve, namePinDashboard, second.String())

	list := r.pinned(t, eve)
	if len(list) != 2 || list[0].ID != second.String() || list[1].ID != first.String() || list[0].Title != "Second" || list[0].FolderTitle != "General" || list[0].PinnedAt == "" {
		t.Fatalf("pinned after pinning twice = %#v, want Second then First once each", list)
	}
	if others := r.pinned(t, bob); len(others) != 0 {
		t.Errorf("another person sees %#v", others)
	}

	if pin(eve, nameUnpinDashboard, first.String()).Pinned {
		t.Error("unpin_dashboard answered pinned true")
	}
	pin(eve, nameUnpinDashboard, first.String())
	if list := r.pinned(t, eve); len(list) != 1 || list[0].ID != second.String() {
		t.Errorf("pinned after unpinning = %#v", list)
	}
}

func TestPinToolsRefuseAServiceAccountKeyAndWhatTheCallerCannotSee(t *testing.T) {
	r := newAccessRig(t).pinning()
	b := r.seedHidden(t)
	gia := r.person(t, "gia", auth.RoleViewer)

	for _, tool := range []string{namePinDashboard, nameUnpinDashboard} {
		if code := errorCode(t, r.call(t, keyCaller, tool, fmt.Sprintf(`{"dashboard_id":%q}`, b.open.String()))); code != codeUserSessionRequired {
			t.Errorf("%s for a key answered %s, want user_session_required", tool, code)
		}
	}
	if code := errorCode(t, r.call(t, keyCaller, nameListPinnedDashboards, `{}`)); code != codeUserSessionRequired {
		t.Errorf("list_pinned_dashboards for a key answered %s, want user_session_required", code)
	}

	for _, tool := range []string{namePinDashboard, nameUnpinDashboard} {
		res := r.call(t, gia, tool, fmt.Sprintf(`{"dashboard_id":%q}`, b.hidden.String()))
		if code := errorCode(t, res); code != codeNotFound {
			t.Errorf("%s of a hidden dashboard answered %s, want not_found", tool, code)
		}
	}
	if list := r.pinned(t, gia); len(list) != 0 {
		t.Errorf("pinned after the refusals = %#v", list)
	}
}

func TestPinToolsNeedTheStarsService(t *testing.T) {
	r := newAccessRig(t)
	id := r.seed(t, "Board")
	person := r.person(t, "eve", auth.RoleEditor)
	for tool, input := range map[string]string{
		namePinDashboard:         fmt.Sprintf(`{"dashboard_id":%q}`, id.String()),
		nameListPinnedDashboards: `{}`,
	} {
		res, err := r.registry.Call(auth.WithPrincipal(t.Context(), person), tool, []byte(input))
		if err != nil || errorCode(t, res) != FailureToolFailed {
			t.Errorf("%s without the stars service: res = %#v, err = %v", tool, res.Content, err)
		}
	}
}
