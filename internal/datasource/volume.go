package datasource

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/logvolume"
)

type VolumeRequest struct {
	Query   Query
	Range   TimeRange
	Counted TimeRange
	Step    time.Duration
}

func (r VolumeRequest) CountedRange() TimeRange {
	if r.Counted == (TimeRange{}) {
		return r.Range
	}
	return r.Counted
}

func (r VolumeRequest) Buckets() int {
	return int(r.Range.To.Sub(r.Range.From) / r.Step)
}

type LogVolumeSource interface {
	LogVolume(ctx context.Context, req VolumeRequest) (*logvolume.Histogram, error)
}
