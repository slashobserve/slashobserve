package cli

import (
	"net/http"
	"strings"
	"testing"
)

const (
	observationPrefix = "/api/v1/observations/" + testObservation
	rcaID             = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	snapshotID        = "99999999-9999-4999-8999-999999999999"
)

func observationEnvelopeJSON() string {
	return `{"observation":` + observationJSON("") + `}`
}

func TestObservationEdit(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		body    string
		wantErr string
	}{
		{"a title", []string{"--title", "Checkout 5xx again"}, `{"title":"Checkout 5xx again"}`, ""},
		{"a range", []string{"--from", "2026-01-01T00:00:00Z", "--to", "2026-01-01T01:00:00Z"}, `{"from":"2026-01-01T00:00:00Z","to":"2026-01-01T01:00:00Z"}`, ""},
		{"a title and a range", []string{"--title", "T", "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-01T01:00:00Z"}, `{"title":"T","from":"2026-01-01T00:00:00Z","to":"2026-01-01T01:00:00Z"}`, ""},
		{"clearing the range", []string{"--clear-range"}, `{"from":null,"to":null}`, ""},
		{"nothing to change", nil, "", "give --title"},
		{"a range with one end", []string{"--from", "2026-01-01T00:00:00Z"}, "", "--from and --to are given together"},
		{"the other end alone", []string{"--to", "2026-01-01T00:00:00Z"}, "", "--from and --to are given together"},
		{"a range and --clear-range", []string{"--from", "2026-01-01T00:00:00Z", "--to", "2026-01-01T01:00:00Z", "--clear-range"}, "", "--clear-range cannot be used"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newManageFake(t, manageReply{http.MethodPatch, observationPrefix, 200, observationEnvelopeJSON()})
			stdout, stderr, code := f.run(append([]string{"o", "edit", testObservation}, tc.args...)...)
			if tc.wantErr != "" {
				if code != 2 || !strings.Contains(stderr, tc.wantErr) || len(f.got) != 0 {
					t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
				}
				return
			}
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			f.wantRequests(t, recordedRequest{http.MethodPatch, observationPrefix, "", tc.body})
			if want := "Updated Checkout 5xx\n"; stdout != want {
				t.Fatalf("stdout %q, want %q", stdout, want)
			}
		})
	}
}

func TestObservationMove(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPost, observationPrefix + "/move", 200, observationEnvelopeJSON()})
	stdout, stderr, code := f.run("o", "move", testObservation, "--folder", manageFolder)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPost, observationPrefix + "/move", "", `{"folder_id":"` + manageFolder + `"}`})
	if want := "Moved Checkout 5xx to folder " + manageFolder + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
	f = newManageFake(t)
	if _, stderr, code := f.run("o", "move", testObservation); code != 2 || !strings.Contains(stderr, "--folder <folder-id> is required") || len(f.got) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
	}
}

func TestObservationReopen(t *testing.T) {
	f := newManageFake(t, manageReply{http.MethodPost, observationPrefix + "/reopen", 200, observationEnvelopeJSON()})
	stdout, stderr, code := f.run("o", "reopen", testObservation)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.wantRequests(t, recordedRequest{http.MethodPost, observationPrefix + "/reopen", "", ""})
	if want := "Reopened Checkout 5xx\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
}

func TestObservationRcas(t *testing.T) {
	listed := `{"rcas":[` +
		`{"id":"` + rcaID + `","revision":2,"title":"Checkout 5xx","summary":"Pool exhausted","observation_state":"resolved","created_by":{"label":"Ada"},"created_at":"2026-01-02T03:04:05Z"},` +
		`{"id":"` + rcaID + `","revision":1,"title":"Checkout 5xx","summary":null,"observation_state":"open","created_by":{"label":"Sam"},"created_at":"2026-01-01T03:04:05Z"}]}`
	shown := `{"rca":{"id":"` + rcaID + `","revision":2,"title":"Checkout 5xx","summary":"Pool exhausted","observation_state":"resolved","created_by":{"label":"Ada"},"created_at":"2026-01-02T03:04:05Z","blocks":[` +
		`{"kind":"text","markdown":"Errors started at 09:12\nthen stopped","width":"full"},` +
		`{"kind":"snapshot","snapshot_id":"` + snapshotID + `","title":"Error rate","panel":{},"width":"half"}]}}`

	t.Run("lists the revisions", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, observationPrefix + "/rcas", 200, listed})
		stdout, stderr, code := f.run("o", "rcas", testObservation)
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, observationPrefix + "/rcas", "", ""})
		for _, want := range []string{
			"REVISION TITLE STATE PUBLISHED BY PUBLISHED",
			"2 Checkout 5xx resolved Ada 2026-01-02T03:04Z",
			"1 Checkout 5xx open Sam 2026-01-01T03:04Z",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
	})
	t.Run("reads one revision", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, observationPrefix + "/rcas/2", 200, shown})
		stdout, stderr, code := f.run("o", "rcas", testObservation, "2")
		if code != 0 || stderr != "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodGet, observationPrefix + "/rcas/2", "", ""})
		want := "Checkout 5xx (revision 2)\nPool exhausted\n\nErrors started at 09:12\nthen stopped\n\nsnapshot " + snapshotID + "\n"
		if stdout != want {
			t.Fatalf("stdout %q, want %q", stdout, want)
		}
	})
	t.Run("prints the body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodGet, observationPrefix + "/rcas/2", 200, shown})
		stdout, _, code := f.run("--json", "o", "rcas", testObservation, "2")
		if code != 0 || !sameJSON(stdout, shown) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
	for name, args := range map[string][]string{
		"a zero revision":         {"o", "rcas", testObservation, "0"},
		"a revision that is text": {"o", "rcas", testObservation, "latest"},
		"three arguments":         {"o", "rcas", testObservation, "1", "2"},
	} {
		t.Run(name+" is a usage error", func(t *testing.T) {
			f := newManageFake(t)
			if _, _, code := f.run(args...); code != 2 || len(f.got) != 0 {
				t.Fatalf("exit %d, requests %v", code, f.got)
			}
		})
	}
}
