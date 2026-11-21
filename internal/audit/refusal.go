package audit

import (
	"encoding/json"
	"maps"
)

var refusalKeys = []string{"needed_role", "needed_access"}

func (w *errorCapture) needs() map[string]any {
	if w.body.Len() == 0 {
		return nil
	}
	var body struct {
		NeededRole   string `json:"needed_role"`
		NeededAccess string `json:"needed_access"`
	}
	if json.Unmarshal(w.body.Bytes(), &body) != nil {
		return nil
	}
	needs := map[string]any{}
	if body.NeededRole != "" {
		needs["needed_role"] = body.NeededRole
	}
	if body.NeededAccess != "" {
		needs["needed_access"] = body.NeededAccess
	}
	return needs
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
