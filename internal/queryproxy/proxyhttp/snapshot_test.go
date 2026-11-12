package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var (
	frozenFrom = time.Date(2025, 10, 9, 8, 0, 0, 0, time.UTC)
	frozenTo   = time.Date(2025, 10, 9, 14, 0, 0, 0, time.UTC)
	viewerUser = auth.Principal{Kind: auth.KindUser, ID: "alice", Email: "alice@example.com", Name: "alice", Role: auth.RoleViewer}
)

func hasKey(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == key || hasKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if hasKey(child, key) {
				return true
			}
		}
	}
	return false
}

func TestFreezeReturnsTheDataBodyWithNoQueryText(t *testing.T) {
	hn := newHarness(t)
	frozen, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("sum(up)"), observations.BlockPanelID, frozenFrom, frozenTo)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.UserScoped {
		t.Error("a panel without user variables is not user scoped")
	}
	var body map[string]any
	if err := json.Unmarshal(frozen.Data, &body); err != nil {
		t.Fatal(err)
	}
	if hasKey(body, "query") || hasKey(body, "text") {
		t.Errorf("the frozen body carries query text: %s", frozen.Data)
	}
	var decoded dataResponse
	if err := json.Unmarshal(frozen.Data, &decoded); err != nil || len(decoded.Frames) == 0 || decoded.Meta.Cached || len(decoded.Meta.Queries) != 1 {
		t.Errorf("body %s, %v", frozen.Data, err)
	}
	hn.ds.mu.Lock()
	got := hn.ds.rangeRange
	hn.ds.mu.Unlock()
	if got.From.Before(frozenFrom.Add(-time.Minute)) || got.To.After(frozenTo.Add(time.Minute)) || got.To.Sub(got.From) < 5*time.Hour {
		t.Errorf("ran %v to %v", got.From, got.To)
	}
	if hn.heldSlots() != 0 {
		t.Errorf("%d slots held after freezing", hn.heldSlots())
	}
}

func TestFreezeMarksADashboardThatReadsTheUser(t *testing.T) {
	hn := newHarness(t)
	frozen, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard(`up{user="${__user.login}"}`), observations.BlockPanelID, frozenFrom, frozenTo)
	if err != nil {
		t.Fatal(err)
	}
	if !frozen.UserScoped {
		t.Error("a panel that reads the user is user scoped")
	}
	if query := hn.ds.lastQuery(); query.User.ID != viewerUser.ID || query.User.Login != viewerUser.Email {
		t.Errorf("the datasource saw user %+v", query.User)
	}
}

func TestFreezeSharesThePerPrincipalSlots(t *testing.T) {
	hn := newHarness(t)
	var releases []func()
	for range maxInFlightPerPrincipal {
		release, ok := hn.h.slots.acquire(viewerUser.ID)
		if !ok {
			t.Fatal("slot refused early")
		}
		releases = append(releases, release)
	}
	_, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("up"), observations.BlockPanelID, frozenFrom, frozenTo)
	if !errors.Is(err, queryproxy.ErrUpstreamBusy) {
		t.Errorf("busy: %v", err)
	}
	for _, release := range releases {
		release()
	}
	if _, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("up"), observations.BlockPanelID, frozenFrom, frozenTo); err != nil {
		t.Errorf("after release: %v", err)
	}
}

func TestFreezeRefusesOversizeData(t *testing.T) {
	for name, bytes := range map[string]int{"over the snapshot limit": observations.MaxSnapshotBytes + 1, "over the proxy limit": maxResponseBytes + 1} {
		t.Run(name, func(t *testing.T) {
			hn := newHarness(t)
			hn.ds.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
				return bigFrames(bytes), nil
			}
			_, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("up"), observations.BlockPanelID, frozenFrom, frozenTo)
			if !errors.Is(err, observations.ErrSnapshotTooLarge) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestFreezeReturnsAQueryFailureAsIs(t *testing.T) {
	hn := newHarness(t)
	hn.ds.failText = map[string]error{"up": errors.New("boom")}
	_, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("up"), observations.BlockPanelID, frozenFrom, frozenTo)
	var qe *queryproxy.QueryError
	if !errors.As(err, &qe) {
		t.Errorf("err = %v", err)
	}
	if _, err := hn.h.Freeze(t.Context(), viewerUser, blockDashboard("up"), "missing", frozenFrom, frozenTo); !errors.Is(err, queryproxy.ErrPanelNotFound) {
		t.Errorf("missing panel: %v", err)
	}
}
