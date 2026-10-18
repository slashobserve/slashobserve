package datasource

import "time"

type Limits struct {
	Timeout   time.Duration `json:"-"`
	MaxSeries int           `json:"maxSeries"`
	MaxPoints int           `json:"maxPoints"`
	MaxRows   int           `json:"maxRows"`
}

var DefaultLimits = Limits{
	Timeout:   30 * time.Second,
	MaxSeries: 1000,
	MaxPoints: 1_000_000,
	MaxRows:   10_000,
}

func (l Limits) Tighten(o Limits) Limits {
	return Limits{
		Timeout:   tighter(l.Timeout, o.Timeout),
		MaxSeries: tighter(l.MaxSeries, o.MaxSeries),
		MaxPoints: tighter(l.MaxPoints, o.MaxPoints),
		MaxRows:   tighter(l.MaxRows, o.MaxRows),
	}
}

func (l Limits) WithDefaults(d Limits) Limits {
	return Limits{
		Timeout:   orDefault(l.Timeout, d.Timeout),
		MaxSeries: orDefault(l.MaxSeries, d.MaxSeries),
		MaxPoints: orDefault(l.MaxPoints, d.MaxPoints),
		MaxRows:   orDefault(l.MaxRows, d.MaxRows),
	}
}

func orDefault[T time.Duration | int](v, d T) T {
	if v == 0 {
		return d
	}
	return v
}

func tighter[T time.Duration | int](base, override T) T {
	if override <= 0 {
		return base
	}
	return min(base, override)
}

func (l Limits) Validate() error {
	switch {
	case l.Timeout <= 0:
		return ConfigErrorf("limit timeout must be positive")
	case l.MaxSeries <= 0:
		return ConfigErrorf("limit maxSeries must be positive")
	case l.MaxPoints <= 0:
		return ConfigErrorf("limit maxPoints must be positive")
	case l.MaxRows <= 0:
		return ConfigErrorf("limit maxRows must be positive")
	}
	return nil
}
