package tools

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/findings"
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

func TestTheSaveReadsTheUnverifiedItemsOfItsCall(t *testing.T) {
	items := []findings.Unverified{{Kind: findings.UnverifiedFigure, Text: "9.3 s"}}
	if got := unverifiedOf(WithUnverified(context.Background(), items)); !slices.Equal(got, items) {
		t.Errorf("unverified = %+v, want %+v", got, items)
	}
	if got := unverifiedOf(context.Background()); got != nil {
		t.Errorf("unverified without a check = %+v, want none", got)
	}
}
