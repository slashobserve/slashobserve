package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func runCLI(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestCompletionScripts(t *testing.T) {
	cases := []struct{ shell, header, mentions string }{
		{"bash", "# bash completion V2 for observe", "__observe_"},
		{"zsh", "#compdef observe", "__observe_"},
		{"fish", "# fish completion for observe", "__observe_"},
	}
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			stdout, stderr, code := runCLI("completion", tc.shell)
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			if first, _, _ := strings.Cut(stdout, "\n"); !strings.HasPrefix(first, tc.header) || !strings.Contains(stdout, tc.mentions) {
				t.Fatalf("script starts %q", first)
			}
		})
	}
}

func TestCompletionRejectsOtherShells(t *testing.T) {
	_, stderr, code := runCLI("completion", "powershell")
	if code != 2 || !strings.Contains(stderr, `unknown command "powershell"`) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestCompletionCompletesCommandNames(t *testing.T) {
	cases := map[string]string{
		"comp": "completion",
		"ann":  "annotate",
		"doc":  "doctor",
		"gra":  "grafana",
		"tra":  "traces",
		"log":  "logs",
	}
	for prefix, want := range cases {
		stdout, _, code := runCLI("__complete", prefix)
		if code != 0 || !slices.Contains(strings.Fields(stdout), want) {
			t.Errorf("%s: exit %d, stdout %q", prefix, code, stdout)
		}
	}
}

func TestTableAlignsColumns(t *testing.T) {
	var stdout bytes.Buffer
	out := &output{out: &stdout, err: &bytes.Buffer{}}
	out.table([]string{"ID", "TITLE", "VERSION"}, [][]string{
		{"a", "Checkout health", "12"},
		{"longer-id", "Shop", "3"},
	})
	rows := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(rows) != 3 {
		t.Fatalf("rows %q", rows)
	}
	for column, header := range []string{"TITLE", "VERSION"} {
		at := strings.Index(rows[0], header)
		for _, row := range rows[1:] {
			if at <= 0 || len(row) <= at || row[at-1] != ' ' || row[at] == ' ' {
				t.Fatalf("column %d is not aligned at %d:\n%s", column+1, at, stdout.String())
			}
		}
	}
}
