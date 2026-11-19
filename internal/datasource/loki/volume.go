package loki

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logvolume"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const volumeNeedsLogQuery = `the volume needs a log query that starts with a stream selector, such as {service_name="checkout"}`

var _ datasource.LogVolumeSource = (*Datasource)(nil)

func VolumeQuery(query string, step time.Duration) (string, error) {
	if !spec.LogQLReturnsLines(query) {
		return "", datasource.Errorf(datasource.ErrInvalidQuery, "%s", volumeNeedsLogQuery)
	}
	return "sum by (" + strings.Join(levelLabels, ", ") + ") (count_over_time(\n" + query + "\n[" + strconv.FormatInt(step.Milliseconds(), 10) + "ms]))", nil
}

func VolumeLevel(labels map[string]string) string {
	level, _ := streamLevel(labels)
	return level
}

func (d *Datasource) LogVolume(ctx context.Context, req datasource.VolumeRequest) (*logvolume.Histogram, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	if req.Step < time.Millisecond {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be at least 1ms")
	}
	text, err := expand(req.Query, req.Range, req.Step)
	if err != nil {
		return nil, err
	}
	req.Query.Expanded(text)
	counting, err := VolumeQuery(text, req.Step)
	if err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	hist := logvolume.New(req.Range.From, req.Step, req.Buckets())
	var pacer datasource.Pacer
	for _, c := range volumeChunks(req.Range, req.Step) {
		win, ok := pacer.Start(ctx, c.To.Sub(c.From))
		if !ok {
			hist.CountedOnlyFrom(c.To)
			break
		}
		frames, err := d.run(win.Ctx, "/loki/api/v1/query_range", url.Values{
			"query": {counting},
			"start": {lokiTime(c.From.Add(req.Step))},
			"end":   {lokiTime(c.To)},
			"step":  {strconv.FormatFloat(req.Step.Seconds(), 'f', -1, 64)},
		}, lim, 0)
		if ranOut := win.End(err); err != nil {
			if !ranOut {
				return nil, err
			}
			hist.CountedOnlyFrom(c.To)
			break
		}
		for _, f := range frames {
			foldVolume(hist, f, req.Step)
		}
	}
	return hist, nil
}

func volumeChunks(r datasource.TimeRange, step time.Duration) []datasource.TimeRange {
	size := step * max(1, (datasource.WidestWindow+step-1)/step)
	var out []datasource.TimeRange
	for to := r.To; to.After(r.From); to = to.Add(-size) {
		out = append(out, datasource.TimeRange{From: maxTime(to.Add(-size), r.From), To: to})
	}
	return out
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func foldVolume(h *logvolume.Histogram, f datasource.Frame, step time.Duration) {
	if len(f.Fields) < 2 {
		return
	}
	kind := logvolume.KindOf(VolumeLevel(f.Labels))
	for i, t := range f.Fields[0].Values {
		at, isTime := t.(time.Time)
		v, isNumber := f.Fields[1].Values[i].(float64)
		if isTime && isNumber {
			h.Add(at.Add(-step), kind, int64(math.Round(v)))
		}
	}
}
