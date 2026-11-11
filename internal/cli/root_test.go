package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestRootNamesEveryCommandAsOneThatMustBeQuoted(t *testing.T) {
	root := (&app{}).command()
	_, list, _ := strings.Cut(root.Long, "command name (")
	list, _, _ = strings.Cut(list, ")")
	listed := strings.Split(list, ", ")

	for _, c := range root.Commands() {
		if !slices.Contains(listed, c.Name()) {
			t.Errorf("the root help does not list the command %q as one a prompt must quote: %q", c.Name(), root.Long)
		}
	}
	if !slices.Contains(listed, "help") {
		t.Errorf("the root help does not list help: %q", root.Long)
	}
}
