package proxyhttp

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/ttlcache"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	resultTTL          = 10 * time.Second
	resultMaxCost      = 64 << 20
	resultMaxEntry     = 4 << 20
	maxResponseBytes   = 32 << 20
	specCacheEntries   = 256
	runTimeout         = time.Minute
	valuesResolution   = 10
	valuesEntryBase    = 64
	valuesItemOverhead = 16
)

type cacheKey [sha256.Size]byte

type specKey struct {
	id      pgtype.UUID
	version int
}

type dataEntry struct {
	frames json.RawMessage
	kind   queryproxy.Kind
	from   time.Time
	to     time.Time
	step   time.Duration

	executedAt time.Time
}

type valuesEntry struct {
	values    []string
	truncated bool
}

type caches struct {
	specs       *ttlcache.Cache[specKey, *spec.Dashboard]
	data        *ttlcache.Cache[cacheKey, dataEntry]
	dataFlights *ttlcache.Group[dataEntry]
	values      *ttlcache.Cache[cacheKey, valuesEntry]
}

func newCaches() *caches {
	return &caches{
		specs:       ttlcache.New[specKey, *spec.Dashboard](0, specCacheEntries),
		data:        ttlcache.New[cacheKey, dataEntry](resultTTL, resultMaxCost),
		dataFlights: ttlcache.NewGroup[dataEntry](runTimeout),
		values:      ttlcache.New[cacheKey, valuesEntry](resultTTL, resultMaxCost),
	}
}

func (e dataEntry) cost() int64 { return int64(len(e.frames)) }

func (e valuesEntry) cost() int64 {
	cost := int64(valuesEntryBase)
	for _, v := range e.values {
		cost += int64(len(v)) + valuesItemOverhead
	}
	return cost
}

type keyBuilder struct{ buf []byte }

func (k *keyBuilder) text(s string) {
	k.buf = binary.BigEndian.AppendUint64(k.buf, uint64(len(s)))
	k.buf = append(k.buf, s...)
}

func (k *keyBuilder) number(n int64) {
	k.buf = binary.AppendVarint(k.buf, n)
}

func (k *keyBuilder) selections(selections map[string]queryproxy.Selection) {
	k.number(int64(len(selections)))
	for _, name := range slices.Sorted(maps.Keys(selections)) {
		sel := selections[name]
		k.text(name)
		if sel.All {
			k.number(1)
		} else {
			k.number(0)
		}
		k.number(int64(len(sel.Values)))
		for _, value := range sel.Values {
			k.text(value)
		}
	}
}

func (k *keyBuilder) sum() cacheKey { return sha256.Sum256(k.buf) }

func dataKey(id pgtype.UUID, version int, panelID string, kind queryproxy.Kind, al alignment, selections map[string]queryproxy.Selection) cacheKey {
	var k keyBuilder
	k.text("data")
	k.text(string(id.Bytes[:]))
	k.number(int64(version))
	k.text(panelID)
	k.text(string(kind))
	k.number(al.rng.From.UnixMilli())
	k.number(al.rng.To.UnixMilli())
	k.number(al.step.Milliseconds())
	k.selections(selections)
	return k.sum()
}

func valuesKey(id pgtype.UUID, version int, name string, from, to time.Time, earlier map[string]queryproxy.Selection) cacheKey {
	var k keyBuilder
	k.text("values")
	k.text(string(id.Bytes[:]))
	k.number(int64(version))
	k.text(name)
	k.number(from.Unix() / valuesResolution)
	k.number(to.Unix() / valuesResolution)
	k.selections(earlier)
	return k.sum()
}
