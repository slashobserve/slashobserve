package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	panelRoute       = "/api/v1/dashboards/:id/versions/:version/panels/:panel_id/data"
	callerQueryLimit = 8
	gaugeEvery       = 5 * time.Second
	serverTimeP95    = 300
)

//go:embed testdata/viewers-dashboard.json
var viewersSpec []byte

type dashboardInfo struct {
	id      string
	version int
	panels  []string
}

func (e *env) dashboard(ctx context.Context) (dashboardInfo, error) {
	e.once.Do(func() { e.dash, e.dashErr = e.createDashboard(ctx) })
	return e.dash, e.dashErr
}

func (e *env) createDashboard(ctx context.Context) (dashboardInfo, error) {
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(viewersSpec, &spec); err != nil {
		return dashboardInfo{}, fmt.Errorf("read the viewers dashboard: %w", err)
	}
	title, _ := json.Marshal(fmt.Sprintf("Load test viewers %s %s", e.cfg.label, time.Now().UTC().Format("20060102T150405Z")))
	spec["title"] = title
	r := e.api.post(ctx, e.editorKey(0), "/api/v1/dashboards", map[string]any{"summary": "load test viewers", "spec": spec})
	if !r.ok() {
		return dashboardInfo{}, fmt.Errorf("create the viewers dashboard: %s", r.describe())
	}
	var out struct {
		Dashboard struct {
			ID string `json:"id"`
		} `json:"dashboard"`
		Version struct {
			Version int `json:"version"`
			Spec    struct {
				Panels []struct {
					ID string `json:"id"`
				} `json:"panels"`
			} `json:"spec"`
		} `json:"version"`
	}
	if err := json.Unmarshal(r.body, &out); err != nil || out.Dashboard.ID == "" || len(out.Version.Spec.Panels) == 0 {
		return dashboardInfo{}, fmt.Errorf("the created dashboard has no id or panels: %s", r.describe())
	}
	info := dashboardInfo{id: out.Dashboard.ID, version: out.Version.Version}
	for _, p := range out.Version.Spec.Panels {
		info.panels = append(info.panels, p.ID)
	}
	return info, nil
}

func track(ctx context.Context, r reply, rec *recorder) {
	if ctx.Err() != nil && r.err != nil {
		return
	}
	r.record(rec)
}

type viewerPhase struct {
	e        *env
	d        dashboardInfo
	distinct bool
	page     *recorder
	data     *recorder
}

func (v *viewerPhase) panelPath(panel string, viewer int) string {
	path := fmt.Sprintf("/api/v1/dashboards/%s/versions/%d/panels/%s/data", v.d.id, v.d.version, panel)
	if v.distinct {
		path += fmt.Sprintf("?from=now-%dm&to=now-%dm", 61+viewer, 1+viewer)
	}
	return path
}

func (v *viewerPhase) panel(ctx context.Context, key, path string) {
	slot := v.e.slots[key]
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-slot }()
	track(ctx, v.e.api.get(ctx, key, path), v.data)
}

func (v *viewerPhase) reload(ctx context.Context, viewer int) {
	key := v.e.viewerKey(viewer)
	track(ctx, v.e.api.get(ctx, key, "/api/v1/dashboards/"+v.d.id), v.page)
	sem := make(chan struct{}, v.e.cfg.panelConcurrency)
	var wg sync.WaitGroup
	for _, p := range v.d.panels {
		sem <- struct{}{}
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()
			v.panel(ctx, key, path)
		}(v.panelPath(p, viewer))
	}
	wg.Wait()
}

func (v *viewerPhase) viewer(ctx context.Context, i int) {
	cfg := v.e.cfg
	if !v.e.sleep(ctx, time.Duration(i)*cfg.ramp/time.Duration(cfg.viewers)) {
		return
	}
	for n := 0; ctx.Err() == nil; n++ {
		v.reload(ctx, i)
		jitter := time.Duration(float64(cfg.refresh) * 0.2 * float64((i*31+n*17)%100) / 100)
		if !v.e.sleep(ctx, cfg.refresh+jitter) {
			return
		}
	}
}

func (v *viewerPhase) run(ctx context.Context) {
	cfg := v.e.cfg
	pctx, cancel := context.WithTimeout(ctx, cfg.ramp+cfg.duration)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < cfg.viewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.viewer(pctx, i)
		}()
	}
	wg.Wait()
}

func runViewers(ctx context.Context, e *env) (result, error) {
	d, err := e.dashboard(ctx)
	if err != nil {
		return result{}, err
	}
	var res result
	for _, distinct := range []bool{false, true} {
		name := "shared range"
		if distinct {
			name = "distinct ranges"
		}
		sec, gates := e.viewerPhase(ctx, d, name, distinct)
		res.sections = append(res.sections, sec)
		res.gates = append(res.gates, gates...)
	}
	return res, nil
}

func (e *env) viewerPhase(ctx context.Context, d dashboardInfo, name string, distinct bool) (section, []gate) {
	phase := &viewerPhase{e: e, d: d, distinct: distinct, page: newRecorder(), data: newRecorder()}
	slotQueries := []gaugeQuery{{key: "slots", name: "slashobserve_datasource_slots_in_use", labels: map[string]string{"datasource": "prom"}}}
	before := e.scrapeAll(ctx)
	tracker, stopTracking := trackGauges(ctx, e.scrapers, slotQueries, gaugeEvery)
	phase.run(ctx)
	stopTracking()
	after := e.scrapeAll(ctx)

	all := merge(phase.page, phase.data)
	title := "Viewers, " + name
	sec := section{Title: title}
	sec.Metrics = append(sec.Metrics, countMetric("viewers", float64(e.cfg.viewers)))
	sec.Metrics = append(sec.Metrics, e.serverTime(before, after)...)
	sec.Metrics = append(sec.Metrics, latencyMetrics("panel data client time", phase.data)...)
	sec.Metrics = append(sec.Metrics, latencyMetrics("dashboard page client time", phase.page)...)
	sec.Metrics = append(sec.Metrics, e.cacheShare(before, after))
	sec.Metrics = append(sec.Metrics, e.slotMetrics(tracker, after)...)
	sec.Metrics = append(sec.Metrics, failureMetrics("viewer", all)...)
	return sec, outcomeGates(title, all)
}

func (e *env) serverTime(before, after [][]sample) []metric {
	delta := histogramDelta(before, after, "slashobserve_http_request_duration_seconds", map[string]string{"route": panelRoute, "status": "200"})
	return []metric{
		secondsMetric("panel data server time p50", histogramQuantile(0.5, delta)),
		secondsMetric("panel data server time p95", histogramQuantile(0.95, delta)).under(serverTimeP95, "300 ms"),
	}
}

func (e *env) cacheShare(before, after [][]sample) metric {
	hits := counterDelta(before, after, "slashobserve_proxy_cache_total", map[string]string{"result": "hit"})
	all := counterDelta(before, after, "slashobserve_proxy_cache_total", nil)
	if all <= 0 {
		return percentMetric("proxy cache hit share", math.NaN())
	}
	return percentMetric("proxy cache hit share", 100*hits/all)
}

func (e *env) slotMetrics(tracker *gaugeMax, after [][]sample) []metric {
	var out []metric
	for i := range e.scrapers {
		out = append(out, countMetric(fmt.Sprintf("server %d prom slots in use, max", i+1), tracker.get(fmt.Sprintf("slots#%d", i))))
		if i < len(after) && after[i] != nil {
			out = append(out, countMetric(fmt.Sprintf("server %d prom slots limit", i+1), sumOf(after[i], "slashobserve_datasource_slots_limit", map[string]string{"datasource": "prom"})))
		}
	}
	return out
}
