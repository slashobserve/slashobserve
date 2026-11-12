package grantmode

import (
	"context"
	"testing"
)

func TestFromDefaultsToDirectWithoutAMode(t *testing.T) {
	if got := From(context.Background()); got != Direct {
		t.Fatalf("From(empty context) = %q, want direct", got)
	}
}

func TestFromReturnsTheModeCarriedByTheContext(t *testing.T) {
	for _, m := range []Mode{Direct, Propose} {
		if got := From(With(context.Background(), m)); got != m {
			t.Errorf("From(With(%q)) = %q", m, got)
		}
	}
}

func TestParseTakesExactlyProposeAsPropose(t *testing.T) {
	for in, want := range map[string]Mode{
		"propose": Propose,
		"direct":  Direct,
		"":        Direct,
		"PROPOSE": Direct,
		"other":   Direct,
	} {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
}
