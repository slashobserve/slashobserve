package pricing

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
)

//go:embed prices.yaml
var defaultPrices []byte

type Price struct {
	Input      *float64 `yaml:"input"`
	Output     *float64 `yaml:"output"`
	CacheRead  *float64 `yaml:"cache_read"`
	CacheWrite *float64 `yaml:"cache_write"`
}

type Tokens struct {
	Input, Output, CacheRead, CacheWrite int64
}

type model struct {
	Price `yaml:",inline"`
	Match []string `yaml:"match"`
}

type Table struct{ models map[string]model }

type document struct {
	Models map[string]model `yaml:"models"`
}

var defaultTable = sync.OnceValue(func() Table {
	table, err := Parse(defaultPrices)
	if err != nil {
		panic(fmt.Sprintf("built-in price table: %v", err))
	}
	return table
})

func Default() Table { return defaultTable() }

func Load(path string) (Table, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the path comes from the operator's settings
	if err != nil {
		return Table{}, err
	}
	table, err := Parse(content)
	if err != nil {
		return Table{}, fmt.Errorf("%s: %w", path, err)
	}
	return table, nil
}

func Parse(content []byte) (Table, error) {
	var doc document
	decoder := yaml.NewDecoder(bytes.NewReader(content), yaml.Strict(), yaml.DisallowUnknownField())
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return Table{}, errors.New("empty file")
		}
		return Table{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Table{}, errors.New("multiple YAML documents are not supported")
	}
	table := Table{models: doc.Models}
	if err := table.validate(); err != nil {
		return Table{}, err
	}
	return table, nil
}

func (t Table) validate() error {
	owner := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(t.models)) {
		entry := t.models[name]
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("models.%s: %w", name, err)
		}
		for _, prefix := range entry.Match {
			if strings.TrimSpace(prefix) == "" {
				return fmt.Errorf("models.%s.match must not contain an empty entry", name)
			}
			if other, ok := owner[prefix]; ok {
				return fmt.Errorf("models.%s.match prefix %q is also listed by %s", name, prefix, other)
			}
			owner[prefix] = name
		}
	}
	return nil
}

func (t Table) Lookup(name string) (Price, bool) {
	if entry, ok := t.models[name]; ok {
		return entry.Price, true
	}
	best, found := "", false
	var price Price
	for _, entry := range t.models {
		for _, prefix := range entry.Match {
			if strings.HasPrefix(name, prefix) && len(prefix) > len(best) {
				best, price, found = prefix, entry.Price, true
			}
		}
	}
	return price, found
}

func (t Table) Cost(name string, n Tokens) (float64, bool) {
	price, ok := t.Lookup(name)
	if !ok {
		return 0, false
	}
	return price.Cost(n)
}

func (p Price) Cost(n Tokens) (float64, bool) {
	usd := 0.0
	for _, part := range []struct {
		tokens int64
		rate   *float64
	}{
		{n.Input, p.Input},
		{n.Output, p.Output},
		{n.CacheRead, p.CacheRead},
		{n.CacheWrite, p.CacheWrite},
	} {
		if part.tokens == 0 {
			continue
		}
		if part.rate == nil {
			return 0, false
		}
		usd += float64(part.tokens) * *part.rate
	}
	return usd / 1e6, true
}

func (p Price) Complete() bool {
	return p.Input != nil && p.Output != nil && p.CacheRead != nil && p.CacheWrite != nil
}

func (p Price) Validate() error {
	for _, rate := range []*float64{p.Input, p.Output, p.CacheRead, p.CacheWrite} {
		if rate != nil && *rate < 0 {
			return errors.New("prices must not be negative")
		}
	}
	return nil
}
