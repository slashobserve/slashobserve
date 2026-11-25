package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxSnapshotSeries       = 10
	maxSnapshotNameBytes    = 120
	maxSnapshotLabelBytes   = 80
	maxSnapshotLabels       = 5
	maxSnapshotSummaryBytes = 1536
	failureSnapshotRead     = "snapshot_read_failed"
	snapshotReadError       = "snapshot data could not be read"
)

type snapshotDataOut struct {
	Series      []snapshotSeriesOut `json:"series,omitempty"`
	SeriesTotal int                 `json:"series_total"`
	OtherFrames int                 `json:"other_frames,omitempty"`
}

type snapshotSeriesOut struct {
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Points int               `json:"points"`
	Min    *float64          `json:"min,omitempty"`
	MinAt  string            `json:"min_at,omitempty"`
	Max    *float64          `json:"max,omitempty"`
	MaxAt  string            `json:"max_at,omitempty"`
	Last   *float64          `json:"last,omitempty"`
}

func snapshotView(out blockViewOut, snap observations.Snapshot) (blockViewOut, error) {
	var data struct {
		Frames []datasource.Frame `json:"frames"`
	}
	if err := json.Unmarshal(snap.Data, &data); err != nil {
		return out, fmt.Errorf("decode snapshot frames: %w", err)
	}
	var panel spec.Panel
	if json.Unmarshal(snap.Panel, &panel) == nil {
		out.Title, out.Type, out.Unit = panel.Title, string(panel.Type), panel.Unit
	}
	out.From, out.To = snap.From.UTC().Format(time.RFC3339), snap.To.UTC().Format(time.RFC3339)
	summary := summarizeSnapshot(data.Frames)
	out.Data = &summary
	return out, nil
}

type seriesMember struct {
	frame datasource.Frame
	field datasource.Field
}

func summarizeSnapshot(frames []datasource.Frame) snapshotDataOut {
	var members []seriesMember
	var out snapshotDataOut
	for _, f := range frames {
		if !f.IsSeries() {
			out.OtherFrames++
			continue
		}
		for _, field := range f.Fields[1:] {
			members = append(members, seriesMember{f, field})
		}
	}
	out.SeriesTotal = len(members)
	var distinct map[string]bool
	if len(members) > 0 {
		distinct = distinguishingLabels(members)
	}
	for _, m := range members[:min(len(members), maxSnapshotSeries)] {
		out.Series = append(out.Series, snapshotSeries(m.frame, m.field, distinct))
	}
	for len(out.Series) > 0 && jsonSize(out) > maxSnapshotSummaryBytes {
		out.Series = out.Series[:len(out.Series)-1]
	}
	return out
}

func distinguishingLabels(members []seriesMember) map[string]bool {
	keys := map[string]bool{}
	for _, m := range members {
		for k := range m.frame.Labels {
			keys[k] = true
		}
	}
	out := map[string]bool{}
	for k := range keys {
		first, firstOK := members[0].frame.Labels[k]
		for _, m := range members[1:] {
			if v, ok := m.frame.Labels[k]; ok != firstOK || v != first {
				out[k] = true
				break
			}
		}
	}
	return out
}

func snapshotSeries(f datasource.Frame, field datasource.Field, distinct map[string]bool) snapshotSeriesOut {
	s := snapshotSeriesOut{Name: clip(snapshotName(f, field), maxSnapshotNameBytes), Points: len(field.Values)}
	keys := make([]string, 0, len(f.Labels))
	for k := range f.Labels {
		if distinct[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	if len(keys) > maxSnapshotLabels {
		keys = keys[:maxSnapshotLabels]
	}
	if len(keys) > 0 {
		s.Labels = make(map[string]string, len(keys))
		for _, k := range keys {
			s.Labels[clip(k, maxSnapshotLabelBytes)] = clip(f.Labels[k], maxSnapshotLabelBytes)
		}
	}
	times := f.Fields[0].Values
	for i, v := range field.Values {
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			continue
		}
		s.Last = &n
		if s.Min == nil || n < *s.Min {
			s.Min, s.MinAt = &n, timeAt(times, i)
		}
		if s.Max == nil || n > *s.Max {
			s.Max, s.MaxAt = &n, timeAt(times, i)
		}
	}
	return s
}

func snapshotName(f datasource.Frame, field datasource.Field) string {
	switch {
	case f.Name != "" && field.Name != "" && field.Name != "Value":
		return f.Name + " " + field.Name
	case f.Name != "":
		return f.Name
	}
	return field.Name
}

func timeAt(times []any, i int) string {
	if i >= len(times) {
		return ""
	}
	switch v := times[i].(type) {
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	case float64:
		return time.UnixMilli(int64(v)).UTC().Format(time.RFC3339)
	}
	return ""
}
