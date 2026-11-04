package llm

const minBuildMaxTokens = 8192

type Tuning struct {
	Effort    string
	MaxTokens int
}

func (t Tuning) ForCall(planning bool) Tuning {
	if planning {
		return Tuning{Effort: stepUp(t.Effort), MaxTokens: t.MaxTokens}
	}
	return Tuning{Effort: t.Effort, MaxTokens: min(t.MaxTokens, max(t.MaxTokens/2, minBuildMaxTokens))}
}

func (t Tuning) CutOffEarly(call Tuning, stop StopReason) bool {
	return stop == StopMaxTokens && call.MaxTokens < t.MaxTokens
}

func stepUp(effort string) string {
	switch effort {
	case "low":
		return "medium"
	case "medium":
		return "high"
	}
	return effort
}
