package audit

import (
	"encoding/json"
	"maps"
)

var refusalKeys = []string{"needed_role", "needed_access"}

type Refusal struct {
	Code         string `json:"code"`
	NeededRole   string `json:"needed_role"`
	NeededAccess string `json:"needed_access"`
}

func ParseRefusal(body []byte) Refusal {
	var refusal Refusal
	if len(body) == 0 || json.Unmarshal(body, &refusal) != nil {
		return Refusal{}
	}
	return refusal
}

func (r Refusal) Needs() map[string]any {
	needs := map[string]any{}
	if r.NeededRole != "" {
		needs["needed_role"] = r.NeededRole
	}
	if r.NeededAccess != "" {
		needs["needed_access"] = r.NeededAccess
	}
	return needs
}

func (w *errorCapture) refusal() Refusal {
	return ParseRefusal(w.body.Bytes())
}

func withNeeds(data, needs map[string]any) map[string]any {
	if len(needs) == 0 {
		return data
	}
	out := maps.Clone(data)
	if out == nil {
		out = map[string]any{}
	}
	maps.Copy(out, needs)
	return out
}
