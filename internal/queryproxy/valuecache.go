package queryproxy

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/ttlcache"
)

const (
	valueCacheTTL        = 10 * time.Second
	valueCacheMaxCost    = 16 << 20
	valueCacheMaxEntry   = 1 << 20
	valueFetchTimeout    = time.Minute
	valueRangeResolution = 10
	valueEntryOverhead   = 64
	valueItemOverhead    = 16
)

type valueCache struct {
	entries *ttlcache.Cache[[sha256.Size]byte, []string]
	flights *ttlcache.Group[[]string]
}

func newValueCache(logger *slog.Logger) *valueCache {
	return &valueCache{
		entries: ttlcache.New[[sha256.Size]byte, []string](valueCacheTTL, valueCacheMaxCost),
		flights: ttlcache.NewGroup[[]string](valueFetchTimeout, logger),
	}
}

func (c *valueCache) get(ctx context.Context, key [sha256.Size]byte, fetch func(context.Context) ([]string, error)) ([]string, error) {
	if values, _, ok := c.entries.Get(key); ok {
		return slices.Clone(values), nil
	}
	values, err := c.flights.Do(ctx, string(key[:]), func(ctx context.Context) ([]string, error) {
		if values, _, ok := c.entries.Get(key); ok {
			return values, nil
		}
		values, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		if cost := valuesCost(values); cost <= valueCacheMaxEntry {
			c.entries.Put(key, values, cost)
		}
		return values, nil
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(values), nil
}

func valuesCost(values []string) int64 {
	cost := int64(valueEntryOverhead)
	for _, v := range values {
		cost += int64(len(v)) + valueItemOverhead
	}
	return cost
}

func valueKey(uid string, q datasource.Query, rng datasource.TimeRange) [sha256.Size]byte {
	h := sha256.New()
	field := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	number := func(n int64) {
		h.Write(binary.AppendVarint(nil, n))
	}
	field(uid)
	field(q.Text)
	use := scanQueryUse(q.Text)
	for _, v := range q.Variables {
		if !use.variables[v.Name] {
			continue
		}
		field(v.Name)
		for _, flag := range []bool{v.All, v.AllValue, v.Adhoc} {
			if flag {
				number(1)
			} else {
				number(0)
			}
		}
		number(int64(len(v.Filters)))
		for _, f := range v.Filters {
			field(f.Key)
			field(f.Op)
			field(f.Value)
		}
		number(int64(len(v.Values)))
		for _, value := range v.Values {
			field(value)
		}
	}
	if use.user {
		field(q.User.ID)
		field(q.User.Login)
		field(q.User.Email)
		field(q.User.Name)
	}
	if use.dashboard {
		field(q.Dashboard)
	}
	number(rng.From.Unix() / valueRangeResolution)
	number(rng.To.Unix() / valueRangeResolution)
	var key [sha256.Size]byte
	h.Sum(key[:0])
	return key
}
