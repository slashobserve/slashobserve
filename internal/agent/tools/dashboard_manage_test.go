package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

func TestMoveDashboardPutsItInTheOtherFolder(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	ops := r.newFolder(t, "Ops")

	res := r.call(t, editorCaller, nameMoveDashboard, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":%q}`, b.open.String(), ops.String()))
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok || saved.DashboardID != b.open.String() || saved.Version != 1 {
		t.Fatalf("move = %s", textOf(t, res))
	}
	dash, _, err := r.service.Get(t.Context(), b.open)
	if err != nil || dash.Folder.Title != "Ops" {
		t.Fatalf("folder after the move = %q, %v", dash.Folder.Title, err)
	}
}

func TestMoveDashboardRefusals(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	ops := r.newFolder(t, "Ops")
	open, hidden := b.open.String(), b.hidden.String()
	eve := r.person(t, "eve", auth.RoleEditor)
	mine := r.call(t, eve, nameListFolders, `{}`).Content.(folderListOut)
	var personal string
	for _, f := range mine.Folders {
		if f.Title == "My Dashboards" {
			personal = f.ID
		}
	}

	tests := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
		text  string
	}{
		{"an unknown folder", editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":"%s"}`, open, "99999999-9999-4999-8999-999999999999"), codeNotFound, "folder not found"},
		{"a viewer", viewerCaller, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":%q}`, open, ops.String()), codeForbiddenRole, ""},
		{"a hidden dashboard", editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":%q}`, hidden, ops.String()), codeNotFound, ""},
		{"a personal folder", eve, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":%q}`, open, personal), codePersonalFolder, ""},
		{"no folder", editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":""}`, open), codeInvalidRequest, "folder_id is required"},
		{"a folder that is not a uuid", editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"folder_id":"x"}`, open), codeInvalidRequest, "folder_id must be a UUID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := r.call(t, tt.by, nameMoveDashboard, tt.input)
			if code := failureCode(t, res); code != tt.code {
				t.Fatalf("answered %s, want %s: %s", code, tt.code, textOf(t, res))
			}
			if tt.text != "" && !strings.Contains(textOf(t, res), tt.text) {
				t.Errorf("answer %s does not say %q", textOf(t, res), tt.text)
			}
		})
	}
}

func TestDuplicateDashboardCopiesItUnderTheGivenTitle(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	ops := r.newFolder(t, "Ops")

	res := r.call(t, editorCaller, nameDuplicateDashboard, fmt.Sprintf(`{"dashboard_id":%q,"title":"Copy of open","folder_id":%q}`, b.open.String(), ops.String()))
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok || saved.DashboardID == b.open.String() || saved.Version != 1 {
		t.Fatalf("duplicate = %s", textOf(t, res))
	}
	copied, err := db.ParseUUID(saved.DashboardID)
	if err != nil {
		t.Fatal(err)
	}
	dash, ver, err := r.service.Get(t.Context(), copied)
	if err != nil || dash.Title != "Copy of open" || ver.Spec.Title != "Copy of open" || dash.Folder.Title != "Ops" {
		t.Fatalf("copy = %q in %q, %v", dash.Title, dash.Folder.Title, err)
	}

	inDefault := r.call(t, keyCaller, nameDuplicateDashboard, fmt.Sprintf(`{"dashboard_id":%q,"title":"Key copy"}`, b.open.String()))
	if got := r.createdIn(t, inDefault); got != "General" {
		t.Errorf("a copy without folder_id landed in %q, want General", got)
	}
}

func TestDuplicateDashboardRefusals(t *testing.T) {
	r := newAccessRig(t)
	b := r.seedHidden(t)
	for name, tt := range map[string]struct {
		by    auth.Principal
		input string
		code  string
	}{
		"a hidden dashboard": {editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"title":"Leak"}`, b.hidden.String()), codeNotFound},
		"an unknown folder":  {editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"title":"Copy","folder_id":"99999999-9999-4999-8999-999999999999"}`, b.open.String()), codeNotFound},
		"a viewer":           {viewerCaller, fmt.Sprintf(`{"dashboard_id":%q,"title":"Copy"}`, b.open.String()), codeForbiddenRole},
		"an empty title":     {editorCaller, fmt.Sprintf(`{"dashboard_id":%q,"title":""}`, b.open.String()), codeInvalidSpec},
	} {
		t.Run(name, func(t *testing.T) {
			if code := failureCode(t, r.call(t, tt.by, nameDuplicateDashboard, tt.input)); code != tt.code {
				t.Errorf("answered %s, want %s", code, tt.code)
			}
		})
	}
	if leaked := r.call(t, editorCaller, nameListDashboards, `{}`).Content.(dashboardListOut); len(leaked.Dashboards) != 1 {
		t.Errorf("dashboards after the refusals = %#v", leaked)
	}
}
