package cli

import (
	"net/http"
	"strings"
	"testing"
)

const sharedThreadID = "dddddddd-0000-4000-8000-000000000001"

func TestShareAnswerPostsAndPrintsTheThread(t *testing.T) {
	const shared = `{"thread_id":"` + sharedThreadID + `","message_id":"` + sharedThreadID + `","replayed":false}`
	call, stdout, stderr, code := runAgainst(t, http.StatusCreated, shared, "share-answer", stoppedID)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if call.method != http.MethodPost || call.path != "/api/v1/observe/"+stoppedID+"/share" || call.body != "" {
		t.Fatalf("got %+v", call)
	}
	if strings.TrimSpace(stdout) != "Shared in thread "+sharedThreadID {
		t.Fatalf("stdout %q", stdout)
	}
	call, stdout, _, _ = runAgainst(t, http.StatusOK, strings.Replace(shared, "false", "true", 1), "share-answer", stoppedID, "--thread", sharedThreadID)
	if call.body != `{"thread_id":"`+sharedThreadID+`"}` {
		t.Fatalf("body %q", call.body)
	}
	if strings.TrimSpace(stdout) != "Already shared in thread "+sharedThreadID {
		t.Fatalf("repeat stdout %q", stdout)
	}
}

func TestShareAnswerErrorLines(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
		args   []string
		code   int
		want   string
	}{
		{"no answer", 409, `{"error":"x","code":"no_answer"}`, nil, 1, "has not finished with an answer to share"},
		{"not private", 409, `{"error":"x","code":"not_shareable"}`, nil, 1, "is not a private question"},
		{"not yours", 404, `{"error":"agent request not found","code":"not_found"}`, nil, 1, "only the person who asked can share its answer"},
		{"bad thread id", 201, `{}`, []string{"--thread", "nope"}, 2, "not a valid id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"share-answer", stoppedID}, tc.args...)
			_, _, stderr, code := runAgainst(t, tc.status, tc.reply, args...)
			if code != tc.code || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q, want %d and %q", code, stderr, tc.code, tc.want)
			}
		})
	}
}
