package llm_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
)

func TestForCall(t *testing.T) {
	tests := []struct {
		name     string
		tuning   llm.Tuning
		planning bool
		want     llm.Tuning
	}{
		{"planning steps low up to medium", llm.Tuning{Effort: "low", MaxTokens: 16000}, true, llm.Tuning{Effort: "medium", MaxTokens: 16000}},
		{"planning steps medium up to high", llm.Tuning{Effort: "medium", MaxTokens: 16000}, true, llm.Tuning{Effort: "high", MaxTokens: 16000}},
		{"planning leaves high alone", llm.Tuning{Effort: "high", MaxTokens: 16000}, true, llm.Tuning{Effort: "high", MaxTokens: 16000}},
		{"planning leaves xhigh alone", llm.Tuning{Effort: "xhigh", MaxTokens: 16000}, true, llm.Tuning{Effort: "xhigh", MaxTokens: 16000}},
		{"planning leaves none alone", llm.Tuning{Effort: "none", MaxTokens: 16000}, true, llm.Tuning{Effort: "none", MaxTokens: 16000}},
		{"planning leaves an empty effort empty", llm.Tuning{MaxTokens: 16000}, true, llm.Tuning{MaxTokens: 16000}},
		{"a save call keeps the effort and halves 16000 to the floor", llm.Tuning{Effort: "medium", MaxTokens: 16000}, false, llm.Tuning{Effort: "medium", MaxTokens: 8192}},
		{"a save call halves 32000", llm.Tuning{Effort: "low", MaxTokens: 32000}, false, llm.Tuning{Effort: "low", MaxTokens: 16000}},
		{"a save call never goes above the configured 4096", llm.Tuning{MaxTokens: 4096}, false, llm.Tuning{MaxTokens: 4096}},
		{"a save call never goes above the configured 1024", llm.Tuning{MaxTokens: 1024}, false, llm.Tuning{MaxTokens: 1024}},
		{"a save call at 64000", llm.Tuning{MaxTokens: 64000}, false, llm.Tuning{MaxTokens: 32000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tuning.ForCall(tt.planning); got != tt.want {
				t.Fatalf("ForCall(%v) = %+v, want %+v", tt.planning, got, tt.want)
			}
		})
	}
}

func TestCutOffEarly(t *testing.T) {
	configured := llm.Tuning{MaxTokens: 16000}
	tests := []struct {
		name string
		call llm.Tuning
		stop llm.StopReason
		want bool
	}{
		{"cut at the lowered cap", llm.Tuning{MaxTokens: 8192}, llm.StopMaxTokens, true},
		{"cut at the configured cap", llm.Tuning{MaxTokens: 16000}, llm.StopMaxTokens, false},
		{"finished under the lowered cap", llm.Tuning{MaxTokens: 8192}, llm.StopToolUse, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configured.CutOffEarly(tt.call, tt.stop); got != tt.want {
				t.Fatalf("CutOffEarly = %v, want %v", got, tt.want)
			}
		})
	}
}
