package loadgen

import (
	"math"
	"time"
)

type Curve string

const (
	CurveDaily Curve = "daily"
	CurveFlat  Curve = "flat"
)

const (
	troughHour = 4.0
	peakHour   = 14.0
	curveSwing = 0.6
)

func (c Curve) Factor(at time.Time) float64 {
	if c == CurveFlat {
		return 1
	}
	h := float64(at.Hour()) + float64(at.Minute())/60 + (float64(at.Second())+float64(at.Nanosecond())/1e9)/3600
	if h >= troughHour && h < peakHour {
		return 1 - curveSwing*math.Cos(math.Pi*(h-troughHour)/(peakHour-troughHour))
	}
	if h < troughHour {
		h += 24
	}
	return 1 + curveSwing*math.Cos(math.Pi*(h-peakHour)/(24-(peakHour-troughHour)))
}
