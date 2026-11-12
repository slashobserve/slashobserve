package runsession

import (
	"math"

	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxUsageTextRunes = 100
	maxMessageRunes   = 500
	maxCostUSD        = 10000.0
	silentFailure     = "the agent did not say why"
)

func cleanUsage(u revisions.Usage) revisions.Usage {
	clean := revisions.Usage{
		Provider:            claims.CleanText(u.Provider, maxUsageTextRunes),
		Model:               claims.CleanText(u.Model, maxUsageTextRunes),
		InputTokens:         max(u.InputTokens, 0),
		OutputTokens:        max(u.OutputTokens, 0),
		CacheReadTokens:     max(u.CacheReadTokens, 0),
		CacheWriteTokens:    max(u.CacheWriteTokens, 0),
		ModelCalls:          max(u.ModelCalls, 0),
		Retries:             max(u.Retries, 0),
		Compactions:         max(u.Compactions, 0),
		Renewals:            max(u.Renewals, 0),
		ModelMS:             max(u.ModelMS, 0),
		MaxSaveOutputTokens: max(u.MaxSaveOutputTokens, 0),
	}
	if cost := u.CostUSD; cost != nil && !math.IsNaN(*cost) && *cost >= 0 && *cost <= maxCostUSD {
		kept := *cost
		clean.CostUSD = &kept
	}
	return clean
}

func (s *State) noteModel(u revisions.Usage) {
	u = cleanUsage(u)
	have := &s.Usage
	if u.Provider != "" {
		have.Provider = u.Provider
	}
	if u.Model != "" {
		have.Model = u.Model
	}
	have.InputTokens = max(have.InputTokens, u.InputTokens)
	have.OutputTokens = max(have.OutputTokens, u.OutputTokens)
	have.CacheReadTokens = max(have.CacheReadTokens, u.CacheReadTokens)
	have.CacheWriteTokens = max(have.CacheWriteTokens, u.CacheWriteTokens)
	have.ModelCalls = max(have.ModelCalls, u.ModelCalls)
	have.Retries = max(have.Retries, u.Retries)
	have.Compactions = max(have.Compactions, u.Compactions)
	have.Renewals = max(have.Renewals, u.Renewals)
	have.ModelMS = max(have.ModelMS, u.ModelMS)
	have.MaxSaveOutputTokens = max(have.MaxSaveOutputTokens, u.MaxSaveOutputTokens)
	if u.CostUSD != nil && (have.CostUSD == nil || *u.CostUSD > *have.CostUSD) {
		have.CostUSD = u.CostUSD
	}
}

func (s *State) usage() revisions.Usage {
	u := s.Usage
	counts := s.Guard.Counts
	u.ToolCalls = counts.ToolCalls
	u.RepairRounds = counts.Repairs
	u.BuildSaves = counts.BuildSaves
	u.Nudges = counts.Nudges
	u.SaveNowRefusals = counts.SaveNowRefusals
	u.StalledTurns = counts.StalledTurns
	u.OnePanelRefusals = counts.OnePanelRefusals
	return u
}

func (s *State) storedUsage(prices pricing.Table) *revisions.Usage {
	u := s.usage()
	if u.ToolCalls == 0 && u.ModelCalls == 0 && u.InputTokens == 0 && u.OutputTokens == 0 && u.CostUSD == nil {
		return nil
	}
	if u.CostUSD == nil {
		if cost, ok := modelCost(prices, u); ok {
			u.CostUSD = &cost
		}
	}
	return &u
}

func modelCost(prices pricing.Table, u revisions.Usage) (float64, bool) {
	return prices.Cost(u.Model, pricing.Tokens{
		Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens,
	})
}

func (s *Service) overCostLimit(u revisions.Usage) bool {
	if s.maxRunCost <= 0 {
		return false
	}
	cost, ok := modelCost(s.prices, u)
	if !ok && u.CostUSD != nil {
		cost, ok = *u.CostUSD, true
	}
	return ok && cost >= s.maxRunCost
}
