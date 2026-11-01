package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionIsPrintedByTheFlagAndTheCommand(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Main(args, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "0.1.0")
			if code != 0 || stdout.String() != "0.1.0\n" || stderr.Len() != 0 {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
			}
		})
	}
}
