package cli

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestARefusedAuditCommandExitsNonZero(t *testing.T) {
	reply := `{"error":"requires the Admin role","code":"forbidden_role"}`
	for _, args := range [][]string{
		{"audit", "search"},
		{"audit", "search", "--all"},
		{"audit", "search", "--follow"},
		{"--json", "audit", "search"},
		{"audit", "export", "--file", filepath.Join(t.TempDir(), "audit.ndjson")},
		{"audit", "show", auditEventID},
		{"audit", "facets", "origin"},
		{"audit", "histogram"},
		{"audit", "aggregate", "--query", "{}"},
		{"audit", "fields"},
		{"audit", "export"},
	} {
		_, stdout, stderr, code := runAudit(t, http.StatusForbidden, nil, reply, args...)
		if code == 0 || stdout != "" || !strings.Contains(stderr, "forbidden_role") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}
