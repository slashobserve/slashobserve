package placement

import (
	"strings"
	"testing"
)

func TestTheBootRefusalCountsItsItemsInTheRightNumber(t *testing.T) {
	one := (&MisplacedError{Datasource: "audit", Census: Census{Personal: 1}}).Error()
	for _, want := range []string{"but 1 dashboard, proposal, observation or saved search outside folders only Admins can read uses it", "which only its owner can move", "move it into"} {
		if !strings.Contains(one, want) {
			t.Errorf("one item: %q lacks %q", one, want)
		}
	}
	many := (&MisplacedError{Datasource: "audit", Census: Census{Personal: 2}}).Error()
	for _, want := range []string{"but 2 dashboards, proposals, observations or saved searches outside folders only Admins can read use it", "which only their owners can move", "move them into"} {
		if !strings.Contains(many, want) {
			t.Errorf("many items: %q lacks %q", many, want)
		}
	}
}
