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

func TestNamesWithoutARangeEndAtTheRunsAsOf(t *testing.T) {
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want datasource.TimeRange
	}{
		{"an as_of run", WithRun(context.Background(), Run{Mode: ModeInvestigate, AsOf: at}), datasource.TimeRange{From: at.Add(-defaultNamesWindow), To: at}},
		{"a live run", WithRun(context.Background(), Run{Mode: ModeInvestigate}), datasource.TimeRange{}},
		{"no run", context.Background(), datasource.TimeRange{}},
	} {
		got, res, ok := namesRange(tc.ctx, "", "")
		if !ok || res.IsError || !got.From.Equal(tc.want.From) || !got.To.Equal(tc.want.To) {
			t.Errorf("%s: namesRange = %v to %v, want %v to %v", tc.name, got.From, got.To, tc.want.From, tc.want.To)
		}
	}
}

func TestAnAsOfRunPinsTheDatasourceClock(t *testing.T) {
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	if got, ok := datasource.PinnedNow(WithRun(context.Background(), Run{AsOf: at})); !ok || !got.Equal(at) {
		t.Fatalf("PinnedNow = %s, %v, want %s", got, ok, at)
	}
	if _, ok := datasource.PinnedNow(WithRun(context.Background(), Run{})); ok {
		t.Fatal("a live run pins the clock")
	}
}
