package spec

import (
	"fmt"
	"time"
)

type TimeOptions struct {
	Location             *time.Location
	WeekStart            WeekStart
	FiscalYearStartMonth int
	NowDelay             time.Duration
}

func (o TimeOptions) location() *time.Location {
	if o.Location == nil {
		return time.UTC
	}
	return o.Location
}

func (o TimeOptions) localNow(now time.Time) time.Time {
	return now.Add(-o.NowDelay).In(o.location())
}

func (w WeekStart) weekday() time.Weekday {
	switch w {
	case WeekStartSunday:
		return time.Sunday
	case WeekStartSaturday:
		return time.Saturday
	default:
		return time.Monday
	}
}

func LoadZone(name string) (*time.Location, error) {
	if name != "" && name != "Local" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc, nil
		}
	}
	return nil, fmt.Errorf("%q is not a known time zone", name)
}

func CheckTimezone(name string) error {
	if name == TimezoneBrowser || name == TimezoneUTC {
		return nil
	}
	_, err := LoadZone(name)
	return err
}

func TimeOptionsFor(d *Dashboard, browserZone string) (TimeOptions, error) {
	loc, err := d.location(browserZone)
	if err != nil {
		return TimeOptions{}, err
	}
	var delay time.Duration
	if d.NowDelay != "" {
		if delay, err = ParseInterval(d.NowDelay); err != nil {
			return TimeOptions{}, fmt.Errorf("nowDelay: %w", err)
		}
	}
	return TimeOptions{
		Location:             loc,
		WeekStart:            d.WeekStart,
		FiscalYearStartMonth: d.FiscalYearStartMonth,
		NowDelay:             delay,
	}, nil
}

func (d *Dashboard) location(browserZone string) (*time.Location, error) {
	switch d.Timezone {
	case "", TimezoneUTC:
		return time.UTC, nil
	case TimezoneBrowser:
		if browserZone == "" {
			return time.UTC, nil
		}
		return LoadZone(browserZone)
	default:
		return LoadZone(d.Timezone)
	}
}
