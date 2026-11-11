package grafanaconn

import (
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type SyncState string

const (
	SyncRunning  SyncState = "running"
	SyncFinished SyncState = "finished"
	SyncFailed   SyncState = "failed"
	SyncCanceled SyncState = "canceled"
)

type SyncAction string

const (
	ActionStart   SyncAction = "start"
	ActionClaim   SyncAction = "claim"
	ActionRelease SyncAction = "release"
	ActionFinish  SyncAction = "finish"
	ActionFail    SyncAction = "fail"
	ActionExhaust SyncAction = "exhaust"
	ActionCancel  SyncAction = "cancel"
)

var SyncMachine = lifecycle.Machine[SyncState, SyncAction]{
	Name:     "Grafana import",
	States:   []SyncState{SyncRunning, SyncFinished, SyncFailed, SyncCanceled},
	Terminal: []SyncState{SyncFinished, SyncFailed, SyncCanceled},
	Meaning: map[SyncState]string{
		SyncRunning:  "A server holds its lease and imports the chosen dashboards.",
		SyncFinished: "Every chosen dashboard was handled, each imported, updated, skipped or failed on its own.",
		SyncFailed:   "It stopped early: Grafana could not be listed, the import could not continue as the person who started it, or its server stopped twice without handing it on.",
		SyncCanceled: "An Admin stopped it; the dashboards it already imported stay.",
	},
	Arrive: map[SyncState]string{
		SyncRunning:  "grafana sync started",
		SyncFinished: "grafana sync finished",
		SyncFailed:   "grafana sync failed",
		SyncCanceled: "grafana sync canceled",
	},
	Moves: []lifecycle.Move[SyncState, SyncAction]{
		{Action: ActionStart, To: []SyncState{SyncRunning}, Who: []lifecycle.Who{lifecycle.Admin}},
		{Action: ActionClaim, From: []SyncState{SyncRunning}, To: []SyncState{SyncRunning}, Event: "grafana sync resumed", Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionRelease, From: []SyncState{SyncRunning}, To: []SyncState{SyncRunning}, Event: "grafana sync released", Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionFinish, From: []SyncState{SyncRunning}, To: []SyncState{SyncFinished}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionFail, From: []SyncState{SyncRunning}, To: []SyncState{SyncFailed}, Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionExhaust, From: []SyncState{SyncRunning}, To: []SyncState{SyncFailed}, Event: "grafana sync lease expired twice", Who: []lifecycle.Who{lifecycle.System}},
		{Action: ActionCancel, From: []SyncState{SyncRunning}, To: []SyncState{SyncCanceled}, Who: []lifecycle.Who{lifecycle.Admin}},
	},
}

func canOf(caller auth.Principal) func(lifecycle.Who) bool {
	return func(w lifecycle.Who) bool { return w == lifecycle.Admin && caller.Role == auth.RoleAdmin }
}

func syncActions(state SyncState, caller auth.Principal) []string {
	out := []string{}
	for _, a := range SyncMachine.Allowed(state, canOf(caller)) {
		out = append(out, string(a))
	}
	return out
}

func uniqueHits(hits []apiHit) []apiHit {
	seen := map[string]bool{}
	var out []apiHit
	for _, h := range hits {
		if seen[h.UID] {
			continue
		}
		seen[h.UID] = true
		out = append(out, h)
	}
	return out
}

func remainingHits(hits []apiHit, recorded map[string]bool) []apiHit {
	var out []apiHit
	for _, h := range hits {
		if !recorded[h.UID] {
			out = append(out, h)
		}
	}
	return out
}
