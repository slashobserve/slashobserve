package runsession

import (
	"cmp"
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	maxUsageTextRunes = 100
	maxMessageRunes   = 500
	maxCostUSD        = 10000.0
	silentFailure     = "the agent did not say why"

	maxReportedCallLimit = 1000
)

var reportedStages = []string{guard.StageOrient, guard.StageHypothesize, guard.StageTest, guard.StageConclude}

func cleanUsage(u revisions.Usage) revisions.Usage {
	clean := revisions.Usage{
		Provider:            claims.CleanText(u.Provider, maxUsageTextRunes),
		Model:               claims.CleanText(u.Model, maxUsageTextRunes),
		InputTokens:         max(u.InputTokens, 0),
		OutputTokens:        max(u.OutputTokens, 0),
		CacheReadTokens:     max(u.CacheReadTokens, 0),
		CacheWriteTokens:    max(u.CacheWriteTokens, 0),
		ModelCalls:          max(u.ModelCalls, 0),
		FailedCalls:         max(u.FailedCalls, 0),
		Retries:             max(u.Retries, 0),
		Compactions:         max(u.Compactions, 0),
		Renewals:            max(u.Renewals, 0),
		ModelMS:             max(u.ModelMS, 0),
		MaxSaveOutputTokens: max(u.MaxSaveOutputTokens, 0),
		Nudges:              max(u.Nudges, 0),
		SaveNowRefusals:     max(u.SaveNowRefusals, 0),
		StalledTurns:        max(u.StalledTurns, 0),
		OnePanelRefusals:    max(u.OnePanelRefusals, 0),
		RepairRounds:        max(u.RepairRounds, 0),
		CallLimit:           min(max(u.CallLimit, 0), maxReportedCallLimit),
		Hypotheses:          keptLedger(u.Hypotheses),
	}
	if slices.Contains(reportedStages, u.StageAtSave) {
		clean.StageAtSave = u.StageAtSave
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
	have.FailedCalls = max(have.FailedCalls, u.FailedCalls)
	have.Retries = max(have.Retries, u.Retries)
	have.Compactions = max(have.Compactions, u.Compactions)
	have.Renewals = max(have.Renewals, u.Renewals)
	have.ModelMS = max(have.ModelMS, u.ModelMS)
	have.MaxSaveOutputTokens = max(have.MaxSaveOutputTokens, u.MaxSaveOutputTokens)
	have.Nudges = max(have.Nudges, u.Nudges)
	have.SaveNowRefusals = max(have.SaveNowRefusals, u.SaveNowRefusals)
	have.StalledTurns = max(have.StalledTurns, u.StalledTurns)
	have.OnePanelRefusals = max(have.OnePanelRefusals, u.OnePanelRefusals)
	have.RepairRounds = max(have.RepairRounds, u.RepairRounds)
	have.CallLimit = max(have.CallLimit, u.CallLimit)
	if len(u.Hypotheses) > 0 {
		have.Hypotheses = u.Hypotheses
	}
	if u.StageAtSave != "" {
		have.StageAtSave = u.StageAtSave
	}
	if u.CostUSD != nil && (have.CostUSD == nil || *u.CostUSD > *have.CostUSD) {
		have.CostUSD = u.CostUSD
	}
}

func (s *State) usage() revisions.Usage {
	u := s.Usage
	counts := s.Guard.Counts
	u.ToolCalls = counts.ToolCalls
	u.RepairRounds = max(counts.Repairs, s.Usage.RepairRounds)
	u.BuildSaves = counts.BuildSaves
	u.Nudges = max(counts.Nudges, s.Usage.Nudges)
	u.SaveNowRefusals = max(counts.SaveNowRefusals, s.Usage.SaveNowRefusals)
	u.StalledTurns = max(counts.StalledTurns, s.Usage.StalledTurns)
	u.OnePanelRefusals = max(counts.OnePanelRefusals, s.Usage.OnePanelRefusals)
	if len(s.Ledger) > 0 {
		u.Hypotheses = s.Ledger
	}
	if loop := s.Guard.Loop; loop != nil {
		u.CallLimit = max(loop.Limit, s.Usage.CallLimit)
		u.StageAtSave = cmp.Or(loop.SavedStage, s.Usage.StageAtSave)
	}
	return u
}

func keptLedger(ledger []revisions.Hypothesis) []revisions.Hypothesis {
	if len(ledger) == 0 || len(ledger) > tools.MaxHypotheses {
		return nil
	}
	size := 0
	for _, h := range ledger {
		size += h.Size()
	}
	if size > tools.MaxLedgerBytes {
		return nil
	}
	return slices.Clone(ledger)
}

func (s *State) storedUsage(prices pricing.Table) *revisions.Usage {
	u := s.usage()
	if u.ToolCalls == 0 && u.ModelCalls == 0 && u.FailedCalls == 0 && u.InputTokens == 0 && u.OutputTokens == 0 && u.CostUSD == nil {
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
