package tools

import (
	"context"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func rangeAsked(ctx context.Context, t *testing.T) datasource.TimeRange {
	t.Helper()
	var asked datasource.TimeRange
	ds := &fakeDS{rangeFn: func(_ context.Context, _ datasource.Query, tr datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		asked = tr
		return nil, nil
	}}
	r := newTestRegistry(t, ds, nil)
	ctx = auth.WithPrincipal(reading(ctx), auth.Principal{Role: auth.RoleAdmin})
	res, err := r.Call(ctx, nameQueryRange, []byte(`{"datasource":"fake","query":"up","from":"now-1h","to":"now"}`))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	return asked
}

func TestARunAsOfAPastTimeReadsTheHourBeforeIt(t *testing.T) {
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	got := rangeAsked(WithRun(context.Background(), Run{Mode: ModeInvestigate, AsOf: at}), t)
	if !got.From.Equal(at.Add(-time.Hour)) || !got.To.Equal(at) {
		t.Fatalf("now-1h to now as of %s asked for %s to %s", at, got.From, got.To)
	}
}

func TestARunWithoutAsOfReadsTheLastHour(t *testing.T) {
	before := time.Now()
	got := rangeAsked(WithRun(context.Background(), Run{Mode: ModeInvestigate}), t)
	if got.To.Before(before) || got.To.After(time.Now()) || got.To.Sub(got.From) != time.Hour {
		t.Fatalf("now-1h to now asked for %s to %s, want the hour ending at the call", got.From, got.To)
	}
}
