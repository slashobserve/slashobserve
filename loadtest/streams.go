package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	streamsPerKey = 5
	streamDrain   = 5 * time.Second
	rssMetric     = "process_resident_memory_bytes"
)

type written struct {
	id string
	at time.Time
}

type delivery struct {
	arrived map[string]time.Time
	dups    int
	opened  bool
}

func (d *delivery) arrive(id string, at time.Time) {
	if _, seen := d.arrived[id]; seen {
		d.dups++
		return
	}
	d.arrived[id] = at
}

func fanOut(deliveries []delivery, writes []written) ([]time.Duration, int) {
	var delays []time.Duration
	lost := 0
	for _, d := range deliveries {
		if !d.opened {
			continue
		}
		for _, w := range writes {
			at, ok := d.arrived[w.id]
			if !ok {
				lost++
				continue
			}
			delays = append(delays, max(at.Sub(w.at), 0))
		}
	}
	return delays, lost
}

func parseActivity(data string) (string, string) {
	var ev struct {
		Type   string `json:"type"`
		Target struct {
			ID string `json:"id"`
		} `json:"target"`
	}
	if json.Unmarshal([]byte(data), &ev) != nil {
		return "", ""
	}
	return ev.Type, ev.Target.ID
}

type streamRun struct {
	e       *env
	client  *apiClient
	d       dashboardInfo
	results []delivery
	rec     *recorder
	opened  chan struct{}
	wg      sync.WaitGroup
}

func (s *streamRun) read(ctx context.Context, i int) {
	res := &s.results[i]
	res.arrived = map[string]time.Time{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.base+"/api/v1/activity/stream?dashboard_id="+s.d.id, nil)
	if err != nil {
		s.rec.Fail(0, "")
		s.opened <- struct{}{}
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.e.viewerKey(i/streamsPerKey))
	req.Header.Set("Accept", "text/event-stream")
	resp, err := s.client.http.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			s.rec.Fail(0, "")
		}
		s.opened <- struct{}{}
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		s.rec.Fail(resp.StatusCode, "")
		s.opened <- struct{}{}
		return
	}
	s.consume(ctx, i, resp)
}

func (s *streamRun) consume(ctx context.Context, i int, resp *http.Response) {
	res := &s.results[i]
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	signalled := false
	var event, data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !signalled {
				s.rec.Fail(0, "")
				s.opened <- struct{}{}
			} else if ctx.Err() == nil {
				s.rec.Fail(0, "stream_closed")
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if event == "ready" && !signalled {
				signalled = true
				res.opened = true
				s.opened <- struct{}{}
			}
			if event == "activity" {
				if typ, id := parseActivity(data); typ == "annotation.created" && id != "" {
					res.arrive(id, time.Now())
				}
			}
			event, data = "", ""
		}
	}
}

func (s *streamRun) open(ctx context.Context) {
	cfg := s.e.cfg
	for i := 0; i < cfg.streams; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if !s.e.sleep(ctx, time.Duration(i)*cfg.ramp/time.Duration(cfg.streams)) {
				s.opened <- struct{}{}
				return
			}
			s.read(ctx, i)
		}()
	}
}

func (s *streamRun) waitOpened(ctx context.Context) {
	timeout := time.NewTimer(s.e.cfg.ramp + time.Minute)
	defer timeout.Stop()
	for n := 0; n < s.e.cfg.streams; n++ {
		select {
		case <-s.opened:
		case <-timeout.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *streamRun) write(ctx context.Context, writes *recorder) []written {
	var out []written
	for n := 0; n < s.e.cfg.events && ctx.Err() == nil; n++ {
		body := map[string]string{"dashboard_id": s.d.id, "text": fmt.Sprintf("load test annotation %d", n)}
		r := s.e.api.post(ctx, s.e.editorKey(0), "/api/v1/annotations", body)
		at := time.Now()
		track(ctx, r, writes)
		if r.ok() {
			var created struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(r.body, &created) == nil && created.ID != "" {
				out = append(out, written{id: created.ID, at: at})
			}
		}
		if !s.e.sleep(ctx, s.e.cfg.eventEvery) {
			break
		}
	}
	return out
}

func runStreams(ctx context.Context, e *env) (result, error) {
	d, err := e.dashboard(ctx)
	if err != nil {
		return result{}, err
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	run := &streamRun{
		e:       e,
		client:  newAPIClient(e.cfg.target.String(), 0),
		d:       d,
		results: make([]delivery, e.cfg.streams),
		rec:     newRecorder(),
		opened:  make(chan struct{}, e.cfg.streams),
	}
	before := e.scrapeAll(ctx)
	run.open(sctx)
	run.waitOpened(ctx)
	afterOpen := e.scrapeAll(ctx)
	writeRec := newRecorder()
	writes := run.write(ctx, writeRec)
	e.sleep(ctx, streamDrain)
	cancel()
	run.wg.Wait()
	return run.summarize(before, afterOpen, writes, writeRec), nil
}

func (s *streamRun) summarize(before, afterOpen [][]sample, writes []written, writeRec *recorder) result {
	delays, lost := fanOut(s.results, writes)
	delay := newRecorder()
	for _, d := range delays {
		delay.Observe(d)
	}
	opened, dups := 0, 0
	for _, r := range s.results {
		if r.opened {
			opened++
		}
		dups += r.dups
	}
	all := merge(s.rec, writeRec)
	sec := section{Title: "Activity streams"}
	sec.Metrics = []metric{
		countMetric("streams requested", float64(s.e.cfg.streams)),
		countMetric("streams opened", float64(opened)),
		countMetric("streams refused or dropped", float64(s.rec.Failures())),
		countMetric("annotations written", float64(len(writes))),
		durationMetric("fan-out delay p50", delay.Quantile(0.5)),
		durationMetric("fan-out delay p95", delay.Quantile(0.95)),
		durationMetric("fan-out delay max", delay.Max()),
		countMetric("events lost", float64(lost)),
		countMetric("events duplicated", float64(dups)),
		bytesMetric("server memory per stream", memoryPerStream(before, afterOpen, opened)),
	}
	sec.Metrics = append(sec.Metrics, failureMetrics("stream and annotation", all)...)
	gates := []gate{
		{Name: "no lost events in streams", Pass: lost == 0, Detail: fmt.Sprintf("%d lost of %d expected", lost, opened*len(writes))},
		{Name: "no duplicated events in streams", Pass: dups == 0, Detail: fmt.Sprintf("%d duplicated", dups)},
		{Name: "every stream opened and stayed open", Pass: s.rec.Failures() == 0 && opened == s.e.cfg.streams, Detail: fmt.Sprintf("%d of %d opened, %d refused or dropped", opened, s.e.cfg.streams, s.rec.Failures())},
	}
	return result{sections: []section{sec}, gates: append(gates, outcomeGates("streams", all)...)}
}

func memoryPerStream(before, after [][]sample, opened int) float64 {
	if opened == 0 {
		return math.NaN()
	}
	return counterDelta(before, after, rssMetric, nil) / float64(opened)
}
