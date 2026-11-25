package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const attributionRunID = "44444444-4444-4444-4444-444444444444"

func actorSeenByATool(ctx context.Context, t *testing.T) actor.Actor {
	t.Helper()
	r := newTestRegistry(t, &fakeDS{}, nil)
	person := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-1111-1111-111111111111", Name: "Ada", Role: auth.RoleAdmin}
	var seen actor.Actor
	r.entries[nameListMetrics] = entry{handle: func(ctx context.Context, _ json.RawMessage) (Result, error) {
		seen = actor.In(ctx, person, "")
		return success("ok", map[string]string{}), nil
	}}
	ctx = origin.With(auth.WithPrincipal(reading(ctx), person), origin.Origin{Kind: origin.Web})
	if res, err := r.Call(ctx, nameListMetrics, []byte(`{}`)); err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	return seen
}

func TestAttributionPutsTheRunOnTheActorOfAWriteInARun(t *testing.T) {
	ctx := WithRun(context.Background(), Run{Mode: ModeCreate, RequestID: attributionRunID})
	got := actorSeenByATool(ctx, t)
	if got.RunID != attributionRunID || got.Label() != "Observe Agent for Ada" {
		t.Fatalf("actor = %+v, label %q", got, got.Label())
	}
}

func TestAttributionLeavesTheActorOfACallOutsideARunAlone(t *testing.T) {
	got := actorSeenByATool(context.Background(), t)
	if got.RunID != "" || got.Label() != "Ada" {
		t.Fatalf("actor = %+v, label %q", got, got.Label())
	}
}

func TestAttributionIgnoresARunWithoutAUsableRequestID(t *testing.T) {
	ctx := WithRun(context.Background(), Run{Mode: ModeCreate, RequestID: "not-a-uuid"})
	if got := actorSeenByATool(ctx, t); got.RunID != "" {
		t.Fatalf("actor = %+v", got)
	}
}

func TestAttributionRecordsTheRunInTheAuditScopeToo(t *testing.T) {
	ctx := audit.Open(context.Background(), audit.Scope{})
	ctx = attributed(auth.WithPrincipal(ctx, auth.Principal{Kind: auth.KindUser, ID: "u"}), nameListMetrics, Run{RequestID: attributionRunID}, true)
	scope, ok := audit.ScopeOf(ctx)
	if !ok || scope.Initiator != audit.InitiatorAgent || scope.RunID.UUID.String() != attributionRunID {
		t.Fatalf("scope = %+v", scope)
	}
	if got := actor.In(ctx, auth.Principal{Kind: auth.KindUser}, "").RunID; got != attributionRunID {
		t.Fatalf("actor run = %q", got)
	}
}
