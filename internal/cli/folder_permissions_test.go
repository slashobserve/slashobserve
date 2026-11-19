package cli

import (
	"net/http"
	"strings"
	"testing"
)

const (
	adaID  = "11111111-aaaa-4aaa-8aaa-111111111111"
	benID  = "22222222-bbbb-4bbb-8bbb-222222222222"
	adaRow = `{"kind":"user","user":{"id":"` + adaID + `","name":"Ada","email":"ada@example.com","role":"editor"},"level":"edit"}`
	benRow = `{"kind":"user","user":{"id":"` + benID + `","name":"Ben","email":"ben@example.com","role":"viewer"},"level":"view"}`
	grpRow = `{"kind":"group","group":"payments","member_count":4,"level":"comment"}`
)

var permissionsPath = "/api/v1/folders/" + manageFolder + "/permissions"

func permissionsJSON(rows ...string) string {
	restricted := "false"
	if len(rows) > 0 {
		restricted = "true"
	}
	return `{"restricted":` + restricted + `,"entries":[` + strings.Join(rows, ",") + `]}`
}

func TestFolderPermissionsShow(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(adaRow, benRow, grpRow)})
	stdout, stderr, code := f.run("folders", "permissions", "show", manageFolder)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodGet, permissionsPath, "", ""})
	for _, want := range []string{
		"KIND NAME EMAIL OR MEMBERS LEVEL",
		"user Ada ada@example.com edit",
		"user Ben ben@example.com view",
		"group payments 4 signed in comment",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestFolderPermissionsShowOnAnOpenFolder(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON()})
	stdout, _, code := f.run("folders", "permissions", "show", manageFolder)
	if code != 0 || !strings.Contains(stdout, "Open to everyone by role") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestFolderPermissionsGrantKeepsTheOtherEntries(t *testing.T) {
	f := newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(adaRow, grpRow)},
		manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON(adaRow, grpRow, benRow)},
	)
	stdout, stderr, code := f.run("folders", "permissions", "grant", manageFolder, "--email", "ben@example.com", "--level", "view")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, permissionsPath, "", ""},
		recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[{"user_id":"` + adaID + `","level":"edit"},{"group":"payments","level":"comment"},{"email":"ben@example.com","level":"view"}]}`},
	)
	if want := "Granted view on folder " + manageFolder + " to ben@example.com\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFolderPermissionsGrantChangesTheLevelOfAnExistingEntry(t *testing.T) {
	for name, args := range map[string][]string{
		"by id":    {"--user", adaID},
		"by email": {"--email", "ADA@example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newManageFake(t,
				manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(adaRow, benRow)},
				manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON()},
			)
			_, stderr, code := f.run(append([]string{"folders", "permissions", "grant", manageFolder, "--level", "admin"}, args...)...)
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			entry := `{"user_id":"` + adaID + `","level":"admin"}`
			if name == "by email" {
				entry = `{"email":"ADA@example.com","level":"admin"}`
			}
			f.wantRequests(t,
				recordedRequest{http.MethodGet, permissionsPath, "", ""},
				recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[` + entry + `,{"user_id":"` + benID + `","level":"view"}]}`},
			)
		})
	}
}

func TestFolderPermissionsGrantToAGroup(t *testing.T) {
	f := newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON()},
		manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON(grpRow)},
	)
	_, stderr, code := f.run("folders", "permissions", "grant", manageFolder, "--group", "payments", "--level", "comment")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, permissionsPath, "", ""},
		recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[{"group":"payments","level":"comment"}]}`},
	)
}

func TestFolderPermissionsRevoke(t *testing.T) {
	f := newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(adaRow, benRow, grpRow)},
		manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON(adaRow, grpRow)},
	)
	stdout, stderr, code := f.run("folders", "permissions", "revoke", manageFolder, "--user", benID)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, permissionsPath, "", ""},
		recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[{"user_id":"` + adaID + `","level":"edit"},{"group":"payments","level":"comment"}]}`},
	)
	if want := "Removed " + benID + " from folder " + manageFolder + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFolderPermissionsRevokeOfTheLastEntryNeedsYes(t *testing.T) {
	f := newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(grpRow)},
		manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON()},
	)
	_, stderr, code := f.run("folders", "permissions", "revoke", manageFolder, "--group", "payments")
	if code != 2 || !strings.Contains(stderr, "rerun with --yes") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodGet, permissionsPath, "", ""})

	f = newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(grpRow)},
		manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON()},
	)
	_, stderr, code = f.run("folders", "permissions", "revoke", manageFolder, "--group", "payments", "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t,
		recordedRequest{http.MethodGet, permissionsPath, "", ""},
		recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[]}`},
	)
}

func TestFolderPermissionsRevokeOfAnUnlistedPersonChangesNothing(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON(adaRow)})
	_, stderr, code := f.run("folders", "permissions", "revoke", manageFolder, "--email", "ben@example.com")
	if code != 1 || !strings.Contains(stderr, "is not on the access list") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodGet, permissionsPath, "", ""})
}

func TestFolderPermissionsOpen(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPut, permissionsPath, 200, permissionsJSON()})
	_, stderr, code := f.run("folders", "permissions", "open", manageFolder)
	if code != 2 || !strings.Contains(stderr, "rerun with --yes") || len(f.got) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
	}
	stdout, stderr, code := f.run("folders", "permissions", "open", manageFolder, "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPut, permissionsPath, "", `{"entries":[]}`})
	if want := "Folder " + manageFolder + " is open to everyone by role\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestFolderPermissionsPrintTheServersRefusal(t *testing.T) {
	f := newManageFake(t,
		manageReply{http.MethodGet, permissionsPath, 200, permissionsJSON()},
		manageReply{http.MethodPut, permissionsPath, 400, `{"error":"no signed-in person has that email","code":"unknown_user"}`},
	)
	_, stderr, code := f.run("folders", "permissions", "grant", manageFolder, "--email", "who@example.com", "--level", "view")
	if code != 1 || !strings.Contains(stderr, "no signed-in person has that email (unknown_user)") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestFolderPermissionsUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"show without an id":          {"folders", "permissions", "show"},
		"show with a bad id":          {"folders", "permissions", "show", "payments"},
		"grant without a target":      {"folders", "permissions", "grant", manageFolder, "--level", "view"},
		"grant with two targets":      {"folders", "permissions", "grant", manageFolder, "--user", adaID, "--group", "x", "--level", "view"},
		"grant without a level":       {"folders", "permissions", "grant", manageFolder, "--group", "x"},
		"grant with an unknown level": {"folders", "permissions", "grant", manageFolder, "--group", "x", "--level", "owner"},
		"grant with a bad user id":    {"folders", "permissions", "grant", manageFolder, "--user", "ada", "--level", "view"},
		"revoke without a target":     {"folders", "permissions", "revoke", manageFolder},
		"open without an id":          {"folders", "permissions", "open", "--yes"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newManageFake(t)
			if _, _, code := f.run(args...); code != 2 || len(f.got) != 0 {
				t.Fatalf("exit %d, requests %v", code, f.got)
			}
		})
	}
}
