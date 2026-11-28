package brief

import "slices"

type Reason string

const (
	Fresh       Reason = ""
	Restart     Reason = "restart"
	OutputLimit Reason = "output_limit"
	Pending     Reason = "pending"
	Conflict    Reason = "conflict"
	Continue    Reason = "continue"
	Renewed     Reason = "renewed"
	CallTimeout Reason = "call_timeout"
	NoFinding   Reason = "no_finding"
	NoAnswer    Reason = "no_answer"
)

var reasons = []Reason{Restart, OutputLimit, Pending, Conflict, Continue, Renewed, CallTimeout, NoFinding, NoAnswer}

func ParseReason(s string) (Reason, bool) {
	if s == "" {
		return Fresh, true
	}
	r := Reason(s)
	return r, slices.Contains(reasons, r)
}

func ResumeReason(t Task) Reason {
	if t.Continues && t.Attempt == 1 {
		return Continue
	}
	return Restart
}
