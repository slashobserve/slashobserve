package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

type Price struct {
	Input      *float64 `yaml:"input"`
	Output     *float64 `yaml:"output"`
	CacheRead  *float64 `yaml:"cache_read"`
	CacheWrite *float64 `yaml:"cache_write"`
}

type pricedModel struct {
	Price `yaml:",inline"`
	Match []string `yaml:"match"`
}

type Pricing struct {
	Models map[string]pricedModel `yaml:"models"`

	override *Price
}

func loadPricing(path string) (Pricing, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's flags
	if err != nil {
		return Pricing{}, err
	}
	var pricing Pricing
	if err := decodeStrict(content, &pricing); err != nil {
		return Pricing{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := pricing.validate(); err != nil {
		return Pricing{}, fmt.Errorf("%s: %w", path, err)
	}
	return pricing, nil
}

func (p Pricing) validate() error {
	owner := map[string]string{}
	for _, model := range slices.Sorted(maps.Keys(p.Models)) {
		entry := p.Models[model]
		if err := entry.validate(); err != nil {
			return fmt.Errorf("models.%s: %w", model, err)
		}
		for _, prefix := range entry.Match {
			if blank(prefix) {
				return fmt.Errorf("models.%s.match must not contain an empty entry", model)
			}
			if other, ok := owner[prefix]; ok {
				return fmt.Errorf("models.%s.match prefix %q is also listed by %s", model, prefix, other)
			}
			owner[prefix] = model
		}
	}
	return nil
}

func (price Price) validate() error {
	for _, rate := range []*float64{price.Input, price.Output, price.CacheRead, price.CacheWrite} {
		if rate != nil && *rate < 0 {
			return errors.New("prices must not be negative")
		}
	}
	return nil
}

func (p Pricing) with(override *Price) Pricing {
	p.override = override
	return p
}

func (p Pricing) lookup(model string) (Price, bool) {
	if p.override != nil {
		return *p.override, true
	}
	if entry, ok := p.Models[model]; ok {
		return entry.Price, true
	}
	best, found := "", false
	var price Price
	for name, entry := range p.Models {
		for _, prefix := range entry.Match {
			if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
				best, price, found = prefix, p.Models[name].Price, true
			}
		}
	}
	return price, found
}

func (p Pricing) cost(u *Usage) *float64 {
	if u == nil {
		return nil
	}
	price, ok := p.lookup(u.Model)
	if !ok {
		return nil
	}
	usd := 0.0
	for _, part := range []struct {
		tokens int64
		rate   *float64
	}{
		{u.InputTokens, price.Input},
		{u.OutputTokens, price.Output},
		{u.CacheReadTokens, price.CacheRead},
		{u.CacheWriteTokens, price.CacheWrite},
	} {
		if part.tokens == 0 {
			continue
		}
		if part.rate == nil {
			return nil
		}
		usd += float64(part.tokens) * *part.rate
	}
	usd /= 1e6
	return &usd
}
