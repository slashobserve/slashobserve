package findings

import (
	"slices"
	"testing"
)

func TestUnverifiedItemsRoundTripAndEmptyStoresNothing(t *testing.T) {
	items := []Unverified{{Kind: UnverifiedFigure, Text: "9.3 s"}, {Kind: UnverifiedOnset, Text: "2026-11-19T22:00:00Z"}}
	raw, err := encodeUnverified(items)
	if err != nil {
		t.Fatal(err)
	}
	if got := UnverifiedOf(raw); !slices.Equal(got, items) {
		t.Errorf("round trip = %+v, want %+v", got, items)
	}
	if raw, _ := encodeUnverified(nil); raw != nil {
		t.Errorf("no items encoded as %s, want NULL", raw)
	}
	if got := UnverifiedOf(nil); got == nil || len(got) != 0 {
		t.Errorf("a NULL column read as %#v, want an empty list", got)
	}
}
