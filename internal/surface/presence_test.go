package surface

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/presence"
)

func TestPresenceEnumsMatchTheStates(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"ServiceAccount", "ConnectedAgent"} {
		if got := enumOf(t, doc, schema, "presence"); !slices.Equal(got, presence.States.Names()) {
			t.Errorf("%s.presence enum = %v, want %v", schema, got, presence.States.Names())
		}
	}
}
