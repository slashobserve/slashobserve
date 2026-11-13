package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const generalFolder = "00000000-0000-4000-8000-000000000001"

func folderJSON(id, title string, general, personal bool, dashboards int, level string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q,"general":%t,"personal":%t,"restricted":false,"dashboard_count":%d,"grantees":[],"grantee_count":0,"created_by":null,"permissions":{},"level":%q}`, id, title, general, personal, dashboards, level)
}

func TestFoldersList(t *testing.T) {
	listed := `{"folders":[` +
		folderJSON(manageFolder, "My Dashboards", false, true, 3, "edit") + "," +
		folderJSON(generalFolder, "General", true, false, 7, "view") + "," +
		folderJSON(manageCopy, "Payments", false, false, 2, "admin") + `]}`
	f := newManageFake(t, manageReply{http.MethodGet, "/api/v1/folders", 200, listed})
	stdout, stderr, code := f.run("folders", "list")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodGet, "/api/v1/folders", "", ""})
	for _, want := range []string{
		"ID TITLE DASHBOARDS LEVEL",
		manageFolder + " My Dashboards (yours) 3 edit",
		generalFolder + " General (General) 7 view",
		manageCopy + " Payments 2 admin",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestFoldersCreate(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPost, "/api/v1/folders", 201, folderJSON(manageFolder, "Team Payments", false, false, 0, "admin")})
	stdout, stderr, code := f.run("folders", "create", "Team", "Payments")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPost, "/api/v1/folders", "", `{"title":"Team Payments"}`})
	if want := "Created folder Team Payments (" + manageFolder + ")\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFoldersCreatePrintsTheServersRefusal(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPost, "/api/v1/folders", 409, `{"error":"a folder with that title exists","code":"folder_exists"}`})
	stdout, stderr, code := f.run("folders", "create", "Payments")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "a folder with that title exists (folder_exists)") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestFoldersRename(t *testing.T) {
	path := "/api/v1/folders/" + manageFolder
	f := newManageFake(t, manageReply{http.MethodPatch, path, 200, folderJSON(manageFolder, "Team Billing", false, false, 0, "admin")})
	stdout, stderr, code := f.run("folders", "rename", manageFolder, "Team", "Billing")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPatch, path, "", `{"title":"Team Billing"}`})
	if want := "Renamed folder to Team Billing\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFoldersDelete(t *testing.T) {
	path := "/api/v1/folders/" + manageFolder
	f := newManageFake(t, manageReply{http.MethodDelete, path, 204, ""})
	stdout, stderr, code := f.run("folders", "delete", manageFolder)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodDelete, path, "", ""})
	if want := "Deleted folder " + manageFolder + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFoldersUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"create without a title":    {"folders", "create"},
		"rename without a title":    {"folders", "rename", manageFolder},
		"rename with a bad id":      {"folders", "rename", "payments", "Billing"},
		"delete with a bad id":      {"folders", "delete", "payments"},
		"an unknown subcommand":     {"folders", "frobnicate"},
		"list with an extra word":   {"folders", "list", "all"},
		"delete without an id":      {"folders", "delete"},
		"delete with two arguments": {"folders", "delete", manageFolder, manageCopy},
	} {
		t.Run(name, func(t *testing.T) {
			f := newManageFake(t)
			if _, _, code := f.run(args...); code != 2 || len(f.got) != 0 {
				t.Fatalf("exit %d, requests %v", code, f.got)
			}
		})
	}
}
