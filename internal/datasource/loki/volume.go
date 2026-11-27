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

const (
	volumeNeedsLogQuery = `the volume needs a log query that starts with a stream selector, such as {service_name="checkout"}`
	volumeChunk         = 24 * time.Hour
	maxCountWindow      = 30 * time.Minute
)

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
	window := countWindow(req.Step)
	counting, err := VolumeQuery(text, window)
	if err != nil {
		return nil, err
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := datasource.Bound(ctx, lim.Timeout)
	defer cancel()
	hist := logvolume.New(req.Range.From, req.Step, req.Buckets())
	var pacer datasource.Pacer
	for _, c := range volumeChunks(req.Range, req.Step, window) {
		win, ok := pacer.Start(ctx, c.To.Sub(c.From))
		if !ok {
			hist.CountedOnlyFrom(c.To)
			break
		}
		frames, err := d.run(win.Ctx, "/loki/api/v1/query_range", url.Values{
			"query": {counting},
			"start": {lokiTime(c.From)},
			"end":   {lokiTime(c.To)},
			"step":  {strconv.FormatFloat(window.Seconds(), 'f', -1, 64)},
		}, lim, 0)
		if ranOut := win.End(err); err != nil {
			if !ranOut {
				return nil, err
			}
			hist.CountedOnlyFrom(c.To)
			break
		}
		for _, f := range frames {
			foldVolume(hist, f, window)
		}
	}
	return hist, nil
}

// Loki splits a range query at its split interval (1 h by default), and a count window wider than that reads several times the data per step.
func countWindow(step time.Duration) time.Duration {
	ms := step.Milliseconds()
	for parts := int64((step + maxCountWindow - 1) / maxCountWindow); parts < ms; parts++ {
		if ms%parts == 0 {
			return time.Duration(ms/parts) * time.Millisecond
		}
	}
	return time.Millisecond
}

func volumeChunks(r datasource.TimeRange, step, window time.Duration) []datasource.TimeRange {
	size := step * max(1, (volumeChunk+step-1)/step)
	if r.To.Sub(r.From) <= size+step {
		return []datasource.TimeRange{r}
	}
	var out []datasource.TimeRange
	for to := r.To; to.After(r.From); to = to.Add(-size) {
		out = append(out, datasource.TimeRange{From: chunkStart(r, to.Add(-size), window), To: to})
	}
	return out
}

func chunkStart(r datasource.TimeRange, from time.Time, window time.Duration) time.Time {
	if from.After(r.From) {
		return from.Add(window)
	}
	return r.From
}

func foldVolume(h *logvolume.Histogram, f datasource.Frame, window time.Duration) {
	if len(f.Fields) < 2 {
		return
	}
	kind := logvolume.KindOf(VolumeLevel(f.Labels))
	for i, t := range f.Fields[0].Values {
		at, isTime := t.(time.Time)
		v, isNumber := f.Fields[1].Values[i].(float64)
		if isTime && isNumber {
			h.Add(at.Add(-window), kind, int64(math.Round(v)))
		}
	}
}
