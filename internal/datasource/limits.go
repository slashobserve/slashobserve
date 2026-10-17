package datasource

import "time"

type Limits struct {
	Timeout   time.Duration `json:"timeout"`
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

func tighter[T time.Duration | int](base, override T) T {
	if override == 0 {
		return base
	}
	return min(base, override)
}

func (l Limits) Validate() error {
	switch {
	case l.Timeout <= 0:
		return Errorf(ErrInvalidQuery, "limit timeout must be positive")
	case l.MaxSeries <= 0:
		return Errorf(ErrInvalidQuery, "limit maxSeries must be positive")
	case l.MaxPoints <= 0:
		return Errorf(ErrInvalidQuery, "limit maxPoints must be positive")
	case l.MaxRows <= 0:
		return Errorf(ErrInvalidQuery, "limit maxRows must be positive")
	}
	return nil
}
