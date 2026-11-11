package main

import (
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
)

type Pricing struct {
	table    pricing.Table
	override *pricing.Price
}

func loadPricing(path string) (Pricing, error) {
	if path == "" {
		return Pricing{table: pricing.Default()}, nil
	}
	table, err := pricing.Load(path)
	if err != nil {
		return Pricing{}, err
	}
	return Pricing{table: table}, nil
}

func (p Pricing) with(override *pricing.Price) Pricing {
	p.override = override
	return p
}

func (p Pricing) cost(u *Usage) *float64 {
	if u == nil {
		return nil
	}
	tokens := pricing.Tokens{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens}
	var usd float64
	var ok bool
	if p.override != nil {
		usd, ok = p.override.Cost(tokens)
	} else {
		usd, ok = p.table.Cost(u.Model, tokens)
	}
	if !ok {
		return nil
	}
	return &usd
}
