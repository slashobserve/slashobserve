package brief

import (
	"strings"
	"testing"
)

func TestRenderPutsTheThreadBeforeTheRequestAndLeavesOtherRunsUnchanged(t *testing.T) {
	const thread = "Ada: why did errors rise?\n\nBo: the deploy was at 09:10"
	const digest = "<discovered>\n- prom metrics: up\n</discovered>"
	const lead = "The request was asked in this thread (earlier messages, oldest first):"
	for name, tc := range renderCases(t) {
		if name == "suggest" {
			continue
		}
		plain, err := Render(statuses, tc.task, tc.views[0], Fresh, digest)
		if err != nil {
			t.Fatal(err)
		}
		task := tc.task
		task.ThreadContext = thread
		got, err := Render(statuses, task, tc.views[0], Fresh, digest)
		if err != nil {
			t.Fatal(err)
		}
		withoutDigest := strings.TrimSuffix(plain.Context, "\n\n"+digest)
		if want := withoutDigest + "\n\n" + lead + "\n" + thread + "\n\n" + digest; got.Context != want {
			t.Fatalf("%s: context\n%s\nwant\n%s", name, got.Context, want)
		}
		if !strings.Contains(got.User, lead+"\n"+thread) || strings.Index(got.User, lead) > strings.Index(got.User, "<request>") {
			t.Fatalf("%s: the thread is not before the request in %q", name, got.User)
		}
		if got.System != plain.System {
			t.Fatalf("%s: the system text changed with a thread", name)
		}
		again, err := Render(statuses, tc.task, tc.views[0], Fresh, digest)
		if err != nil || again != plain {
			t.Fatalf("%s: a run with no thread context renders differently (err %v)", name, err)
		}
	}
}
