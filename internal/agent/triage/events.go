package triage

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const maxBurstsPerService = 3

func burstStarts(series []Series, label, uid string) []Event {
	var events []Event
	for _, s := range series {
		name := s.Labels[label]
		if name == "" {
			continue
		}
		points := slices.Clone(s.Points)
		slices.SortFunc(points, func(a, b Point) int { return a.At.Compare(b.At) })
		var starts []Event
		for i, p := range points {
			if p.Value <= 0 || (i > 0 && points[i-1].Value > 0 && p.At.Sub(points[i-1].At) <= gapOf(points)) {
				continue
			}
			starts = append(starts, Event{Kind: EventDeployLine, Source: uid, Service: name, At: p.At, Text: fmt.Sprintf("%.0f lines in the first step", p.Value)})
		}
		events = append(events, starts[max(0, len(starts)-maxBurstsPerService):]...)
	}
	return events
}

func gapOf(points []Point) time.Duration {
	gap := time.Duration(0)
	for i := 1; i < len(points); i++ {
		d := points[i].At.Sub(points[i-1].At)
		if gap == 0 || d < gap {
			gap = d
		}
	}
	return gap
}

func flagFirstSeen(series []Series, uid string, from time.Time) []Event {
	var events []Event
	for _, s := range series {
		key := s.Labels[logFlagKeys[0]]
		variant := cmp.Or(s.Labels[logFlagKeys[1]], s.Labels[logFlagKeys[2]])
		if key == "" {
			continue
		}
		i := slices.IndexFunc(s.Points, func(p Point) bool { return p.Value > 0 })
		if i < 0 {
			continue
		}
		events = append(events, flagEvent(uid, key, variant, s.Points[i].At, from))
	}
	slices.SortFunc(events, func(a, b Event) int { return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.Text, b.Text)) })
	return events
}

func flagEvent(uid, key, variant string, at, from time.Time) Event {
	text := fmt.Sprintf("%s variant %s first seen", key, cmp.Or(variant, "(none)"))
	if !at.After(from.Add(time.Minute)) {
		text = fmt.Sprintf("%s variant %s already evaluated when the read began", key, cmp.Or(variant, "(none)"))
	}
	return Event{Kind: EventFlag, Source: uid, At: at, Text: text}
}

func (sw *sweep) traceFlags(ctx context.Context) {
	srcs := sw.sources(datasource.SignalTraces, "tempo")
	if len(srcs) == 0 {
		sw.skip("feature flag evaluations in spans", "", "no Tempo store lists services, and span events are read on Tempo only")
		return
	}
	src := srcs[0]
	rng, _ := sw.eventRange()
	step := "feature flag evaluations in spans"
	var keys TraceStats
	if !sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
		keys, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: flagQuery(), GroupBy: flagKeyAttr, Range: rng, Buckets: 1})
		return err
	}) {
		return
	}
	type pair struct{ key, attr, variant string }
	var pairs []pair
	for _, k := range topGroups(keys.Groups, maxFlagKeys) {
		for _, attr := range flagVariants {
			var variants TraceStats
			if !sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
				variants, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: flagQuery(attrEquals(flagKeyAttr, k)), GroupBy: attr, Range: rng, Buckets: 1})
				return err
			}) {
				return
			}
			found := topGroups(variants.Groups, maxFlagPairs)
			for _, v := range found {
				pairs = append(pairs, pair{k, attr, v})
			}
			if len(found) > 0 {
				break
			}
		}
	}
	if len(pairs) > 0 {
		sw.signal("feature flag evaluations in span events on " + src.UID)
	}
	for _, p := range pairs[:min(len(pairs), maxFlagPairs)] {
		var stats TraceStats
		if !sw.call(ctx, step, src.UID, func(ctx context.Context) (err error) {
			stats, err = sw.r.TraceStats(ctx, src.UID, TraceStatsInput{Query: flagQuery(attrEquals(flagKeyAttr, p.key), attrEquals(p.attr, p.variant)), Range: rng, Buckets: traceBuckets})
			return err
		}) {
			return
		}
		if i := slices.IndexFunc(stats.Buckets, func(b TraceBucket) bool { return b.Spans > 0 }); i >= 0 {
			sw.addEvents([]Event{flagEvent(src.UID, p.key, p.variant, stats.Buckets[i].Start, rng.From)})
		}
	}
}

func topGroups(groups []TraceGroup, n int) []string {
	kept := slices.DeleteFunc(slices.Clone(groups), func(g TraceGroup) bool { return g.Value == "" || g.Spans == 0 })
	slices.SortStableFunc(kept, func(a, b TraceGroup) int { return cmp.Or(cmp.Compare(b.Spans, a.Spans), cmp.Compare(a.Value, b.Value)) })
	out := make([]string, 0, n)
	for _, g := range kept[:min(len(kept), n)] {
		out = append(out, g.Value)
	}
	return out
}

func (sw *sweep) patterns(ctx context.Context, focus []move) {
	if len(focus) == 0 {
		return
	}
	src, ok := sw.lokiSource("log patterns")
	if !ok {
		return
	}
	label := sw.field(src.UID, datasource.SignalLogs)
	if !labelName.MatchString(label) {
		sw.skip("log patterns", src.UID, "the service field is not a stream label")
		return
	}
	sw.signal("log patterns per candidate service at every level on " + src.UID + ", the window's half against the half before it")
	var tasks []func(context.Context)
	for _, m := range focus {
		rng, _ := sw.rung(finest(m))
		tasks = append(tasks, func(ctx context.Context) {
			var got []PatternTrend
			if sw.call(ctx, "log patterns", src.UID, func(ctx context.Context) (err error) {
				got, err = sw.r.Patterns(ctx, src.UID, lokiSelector(label, m.service), rng)
				return err
			}) {
				sw.mu.Lock()
				sw.findings.Patterns = append(sw.findings.Patterns, changedPatterns(got, src.UID, m.service)...)
				sw.mu.Unlock()
			}
		})
	}
	sw.parallel(ctx, tasks)
}

func changedPatterns(found []PatternTrend, uid, service string) []Pattern {
	var out []Pattern
	for _, p := range found {
		half := len(p.Trend) / 2
		var before, during int64
		for i, n := range p.Trend {
			if i < half {
				before += n
			} else {
				during += n
			}
		}
		isNew := before == 0 && during >= minNewPattern
		grew := before > 0 && during >= 2*before && during-before >= minGrownPattern
		if isNew || grew {
			out = append(out, Pattern{Source: uid, Service: service, Level: p.Level, Text: p.Text, Window: during, Baseline: before})
		}
	}
	return out
}
