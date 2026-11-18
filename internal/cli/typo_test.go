package cli

import (
	"strings"
	"testing"
)

func TestAMistypedCommandIsRefusedBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"one bare word", []string{"x"}, `"x" is one word`},
		{"a command with a missing letter", []string{"dashbords", "list"}, `did you mean "dashboards"`},
		{"a command with a swapped letter", []string{"helth"}, "is one word"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			call, _, stderr, code := runAgainst(t, 200, `{}`, tc.args...)
			if code != 2 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q, want exit 2 and %q", code, stderr, tc.want)
			}
			if call.method != "" {
				t.Fatalf("the CLI sent %+v for a probable typo", call)
			}
		})
	}
}

func TestWordsThatAreNotNearACommandStayAPrompt(t *testing.T) {
	root := (&app{}).command()
	for _, args := range [][]string{
		{"checkout", "p99", "latency"},
		{"why", "did", "checkout", "errors", "rise"},
		{"dashbords list"},
		{"x y"},
		{"log", "errors"},
	} {
		if err := probableTypo(root, args); err != nil {
			t.Errorf("%q refused: %v", args, err)
		}
	}
}

func TestTheObservationsCommandAnswersToItsLongName(t *testing.T) {
	stdout, _, code := runCLI("observations", "--help")
	if code != 0 || !strings.Contains(stdout, "o, observations") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}
