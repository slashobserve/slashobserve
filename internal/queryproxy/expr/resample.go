package expr

import (
	"math"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type bucket struct {
	count         int
	sum, min, max float64
	last          float64
}

func (b *bucket) add(v float64) {
	if b.count == 0 || v < b.min {
		b.min = v
	}
	if b.count == 0 || v > b.max {
		b.max = v
	}
	b.count++
	b.sum += v
	b.last = v
}

func (b bucket) down(downsampler string) float64 {
	switch downsampler {
	case "min":
		return b.min
	case "max":
		return b.max
	case "sum":
		return b.sum
	case "last":
		return b.last
	}
	return b.sum / float64(b.count)
}

func resample(e spec.Expression, in []series, rng datasource.TimeRange, limits datasource.Limits) ([]series, error) {
	window, err := spec.ParseInterval(e.Window)
	if err != nil {
		return nil, invalid("resample: %v", err)
	}
	if err := rng.Validate(); err != nil {
		return nil, invalid("resample: the time range is not usable")
	}
	buckets := int64(rng.To.Sub(rng.From)/window) + 1
	if buckets > int64(limits.MaxPoints) {
		return nil, datasource.Errorf(datasource.ErrLimitExceeded, "points limit exceeded: resampling every %s would produce more than %d points", e.Window, limits.MaxPoints)
	}
	out := make([]series, len(in))
	for i, s := range in {
		if !s.timed {
			return nil, invalid("resample: input must be a time series, not a single number")
		}
		out[i] = resampleSeries(e, s, rng, window, int(buckets))
	}
	return out, nil
}

func resampleSeries(e spec.Expression, s series, rng datasource.TimeRange, window time.Duration, n int) series {
	from := rng.From
	groups := make([]bucket, n)
	for i, t := range s.times {
		if t.Before(from) || t.After(rng.To) || isNull(s.values[i]) {
			continue
		}
		k := int(t.Sub(from) / window)
		if k >= n {
			continue
		}
		groups[k].add(s.values[i])
	}
	out := series{name: s.name, labels: s.labels, timed: true, times: make([]time.Time, n), values: make([]float64, n)}
	for k := range groups {
		out.times[k] = from.Add(time.Duration(k) * window)
		if groups[k].count > 0 {
			out.values[k] = groups[k].down(e.Downsampler)
		} else {
			out.values[k] = math.NaN()
		}
	}
	fill(out.values, groups, e.Upsampler)
	return out
}

func fill(values []float64, groups []bucket, upsampler string) {
	switch upsampler {
	case "pad":
		for k := 1; k < len(values); k++ {
			if groups[k].count == 0 {
				values[k] = values[k-1]
			}
		}
	case "backfill":
		for k := len(values) - 2; k >= 0; k-- {
			if groups[k].count == 0 {
				values[k] = values[k+1]
			}
		}
	}
}
