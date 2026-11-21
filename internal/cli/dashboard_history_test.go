package cli

import (
	"strings"
	"testing"
)

func TestRevisionsListPagesAndShowsOne(t *testing.T) {
	row := `{"id":"` + stoppedID + `","intent":"apply","prompt":"add a panel\nfor errors","state":"succeeded","base_version":2,"result_version":3,"error":null,"created_at":"2026-01-01T00:00:00Z"}`
	server, seen := recordingServer(t, `{"revisions":[`+row+`],"next_cursor":"c2"}`)
	stdout, stderr, code := runWith(server, "dashboards", "revisions", testDashboard, "--limit", "10")
	if code != 0 || seen.path != "/api/v1/dashboards/"+testDashboard+"/revisions" || seen.query != "limit=10" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "add a panel") || !strings.Contains(stderr, "More: --cursor c2") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}

	server, seen = recordingServer(t, `{"revision":`+row+`}`)
	stdout, _, code = runWith(server, "dashboards", "revisions", testDashboard, stoppedID)
	if code != 0 || seen.path != "/api/v1/dashboards/"+testDashboard+"/revisions/"+stoppedID || !strings.Contains(stdout, "Saved version  3") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
}

func TestDashboardViewersPrintsWhoIsLooking(t *testing.T) {
	server, seen := recordingServer(t, `{"viewers":[{"id":"`+testDashboard+`","name":"Grace"}]}`)
	stdout, _, code := runWith(server, "dashboards", "viewers", testDashboard)
	if code != 0 || seen.path != "/api/v1/dashboards/"+testDashboard+"/viewers" || seen.method != "GET" || !strings.Contains(stdout, "Grace") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
}

func TestDashboardTagsPrintsCounts(t *testing.T) {
	server, seen := recordingServer(t, `{"tags":[{"tag":"prod","dashboards":4}]}`)
	stdout, _, code := runWith(server, "dashboards", "tags")
	if code != 0 || seen.path != "/api/v1/dashboards/tags" || seen.query != "" || !strings.Contains(stdout, "prod") || !strings.Contains(stdout, "4") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
}

func TestDashboardTagsSendsExcludeAuditOnlyWhenAsked(t *testing.T) {
	server, seen := recordingServer(t, `{"tags":[]}`)
	if _, _, code := runWith(server, "dashboards", "tags", "--exclude-audit"); code != 0 || seen.query != "exclude_audit=true" {
		t.Fatalf("exit %d, query %q, want exclude_audit=true", code, seen.query)
	}
}
