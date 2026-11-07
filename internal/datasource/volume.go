package datasource

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/logvolume"
)

type VolumeRequest struct {
	Query Query
	Range TimeRange
	Step  time.Duration
}

func (r VolumeRequest) Buckets() int {
	return int(r.Range.To.Sub(r.Range.From) / r.Step)
}

type LogVolumeSource interface {
	LogVolume(ctx context.Context, req VolumeRequest) (*logvolume.Histogram, error)
}
