package tools

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestAResolvedRangeIsNotedForTheCallThatAsked(t *testing.T) {
	asOf := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	ctx, ranges := NotingRanges(WithRun(context.Background(), Run{Mode: ModeInvestigate, AsOf: asOf}))
	if _, _, ok := resolveRunRange(ctx, "now-2h", "now"); !ok {
		t.Fatal("the range did not resolve")
	}
	if _, _, ok := resolveRunRange(ctx, "not a time", "now"); ok {
		t.Fatal("a bad range resolved")
	}
	want := []datasource.TimeRange{{From: asOf.Add(-2 * time.Hour), To: asOf}}
	if got := ranges(); !slices.Equal(got, want) {
		t.Errorf("ranges = %+v, want %+v", got, want)
	}
	if _, _, ok := resolveRunRange(context.Background(), "now-1h", "now"); !ok {
		t.Error("a call outside a noting context failed to resolve")
	}
}
