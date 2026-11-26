package contextsources

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestFindingToolsRanksTheApprovedToolsOfARegisteredSource(t *testing.T) {
	r := newRig(t, readOnlyTool("search_code", "Search the source code for a text"), readOnlyTool("read_file", "Read the contents of a file"))
	r.approvedSource(t, "docs", "s3cret-value")

	found, err := r.service.Find(accessFor(t, r.admin), "search the code", "", 8)
	if err != nil {
		t.Fatal(err)
	}
	if found.Total != 1 || len(found.Matches) != 1 {
		t.Fatalf("found = %+v", found)
	}
	match := found.Matches[0]
	if match.Source != "docs" || match.SourceName != "Docs" || match.Name != "search_code" || match.Arguments != "query (string, required)" || !strings.Contains(string(match.InputSchema), `"query"`) {
		t.Errorf("match = %+v", match)
	}

	named, err := r.service.Find(accessFor(t, r.admin), "read a file", "docs", 8)
	if err != nil || len(named.Matches) == 0 || named.Matches[0].Name != "read_file" {
		t.Fatalf("named = %+v err = %v", named, err)
	}
	limited, err := r.service.Find(accessFor(t, r.admin), "file code", "", 1)
	if err != nil || len(limited.Matches) != 1 || limited.Total != 2 {
		t.Fatalf("limited = %+v err = %v", limited, err)
	}
}

func TestFindingToolsNeverShowsAnAdminsOnlySourceToAViewer(t *testing.T) {
	r := newRig(t, readOnlyTool("search_code", "Search the source code"))
	r.approvedSource(t, "docs", "s3cret-value")
	viewer := auth.Principal{Kind: auth.KindUser, ID: "viewer", Name: "vi", Role: auth.RoleViewer}

	found, err := r.service.Find(accessFor(t, viewer), "search code", "", 8)
	if err != nil || found.Total != 0 || len(found.Matches) != 0 {
		t.Fatalf("found = %+v err = %v", found, err)
	}
	_, err = r.service.Find(accessFor(t, viewer), "search code", "docs", 8)
	if failureOf(t, err).Code != tools.ContextCodeSourceNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestFindingToolsInAnUnknownSourceIsRefused(t *testing.T) {
	r := newRig(t, readOnlyTool("search_code", "Search the source code"))
	r.approvedSource(t, "docs", "s3cret-value")
	_, err := r.service.Find(accessFor(t, r.admin), "search code", "nowhere", 8)
	if failureOf(t, err).Code != tools.ContextCodeSourceNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestFindingToolsSkipsAToolThatChangedSinceItWasApproved(t *testing.T) {
	r := newRig(t, readOnlyTool("search_code", "Search the source code"))
	r.approvedSource(t, "docs", "s3cret-value")
	r.fake.setTools(readOnlyTool("search_code", "Search the source code and do something else"))
	r.discover(t, "docs")

	found, err := r.service.Find(accessFor(t, r.admin), "search code", "", 8)
	if err != nil || found.Total != 0 {
		t.Fatalf("found = %+v err = %v", found, err)
	}
}
