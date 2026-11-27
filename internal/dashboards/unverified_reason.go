package dashboards

import (
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type UnverifiedReason string

const (
	ReasonEmpty    UnverifiedReason = "empty"
	ReasonTimeout  UnverifiedReason = "timeout"
	ReasonCarried  UnverifiedReason = "carried"
	ReasonRestored UnverifiedReason = "restored"
	ReasonImported UnverifiedReason = "imported"
)

var UnverifiedReasons = []UnverifiedReason{ReasonEmpty, ReasonTimeout, ReasonCarried, ReasonRestored, ReasonImported}

const (
	VerificationUnverified = "unverified"
	VerificationVerified   = "verified"
)

var VerificationStates = lifecycle.Derived{Name: "panel and variable verification", States: []lifecycle.DerivedState{
	{Name: VerificationUnverified, When: "The version lists it in `unverified`: its query failed or returned nothing when the version was saved and the save kept it, with a `reason` of `empty` (it returned no data), `timeout` (it timed out and the save did not change it), `carried` (the parent version already listed it and it is unchanged), `restored` (the restored version listed it) or `imported` (a Grafana import keeps every failing panel)."},
	{Name: VerificationVerified, When: "None of the above; its query ran when the version was saved, or when the last version that changed it was."},
}}

func (u Unverified) because(reason UnverifiedReason) Unverified {
	return u.becauseOf(func([]spec.Issue) UnverifiedReason { return reason })
}

func (u Unverified) becauseOf(reason func([]spec.Issue) UnverifiedReason) Unverified {
	out := Unverified{}
	for _, p := range u.Panels {
		p.Reason = reason(p.Issues)
		out.Panels = append(out.Panels, p)
	}
	for _, v := range u.Variables {
		v.Reason = reason(v.Issues)
		out.Variables = append(out.Variables, v)
	}
	return out
}

func unchangedFailureReason(issues []spec.Issue) UnverifiedReason {
	if len(issues) > 0 && onlyCodes(issues, queryproxy.CodeQueryEmpty) {
		return ReasonEmpty
	}
	return ReasonTimeout
}
