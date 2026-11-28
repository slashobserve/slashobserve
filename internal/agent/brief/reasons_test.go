package brief

import "testing"

func TestParseReasonKnowsEveryReasonAndNothingElse(t *testing.T) {
	for _, s := range []string{"", "restart", "output_limit", "pending", "conflict", "continue", "renewed", "call_timeout", "no_finding", "no_answer"} {
		if got, ok := ParseReason(s); !ok || string(got) != s {
			t.Fatalf("ParseReason(%q) = %q, %v", s, got, ok)
		}
	}
	for _, s := range []string{"fresh", "Restart", "unknown", " "} {
		if _, ok := ParseReason(s); ok {
			t.Fatalf("ParseReason(%q) accepted", s)
		}
	}
}

func TestResumeReasonIsContinueOnlyForTheFirstAttemptOfAContinuation(t *testing.T) {
	for _, tc := range []struct {
		name string
		task Task
		want Reason
	}{
		{"first attempt of a continuation", Task{Continues: true, Attempt: 1}, Continue},
		{"second attempt of a continuation", Task{Continues: true, Attempt: 2}, Restart},
		{"first attempt of an ordinary request", Task{Attempt: 1}, Restart},
		{"second attempt of an ordinary request", Task{Attempt: 2}, Restart},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResumeReason(tc.task); got != tc.want {
				t.Fatalf("ResumeReason = %q, want %q", got, tc.want)
			}
		})
	}
}
