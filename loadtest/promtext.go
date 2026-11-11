package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sample struct {
	name   string
	labels map[string]string
	value  float64
}

func parseLabels(s string) (map[string]string, bool) {
	labels := map[string]string{}
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 || eq+1 >= len(s) || s[eq+1] != '"' {
			return nil, false
		}
		key := strings.TrimSpace(s[:eq])
		var value strings.Builder
		i := eq + 2
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				switch s[i+1] {
				case 'n':
					value.WriteByte('\n')
				default:
					value.WriteByte(s[i+1])
				}
				i += 2
				continue
			}
			if c == '"' {
				closed = true
				i++
				break
			}
			value.WriteByte(c)
			i++
		}
		if !closed {
			return nil, false
		}
		labels[key] = value.String()
		s = strings.TrimLeft(s[i:], ", ")
	}
	return labels, true
}

func parseLine(line string) (sample, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return sample{}, false
	}
	var name, rest string
	labels := map[string]string{}
	if open := strings.IndexByte(line, '{'); open >= 0 {
		end := strings.LastIndexByte(line, '}')
		if end < open {
			return sample{}, false
		}
		parsed, ok := parseLabels(line[open+1 : end])
		if !ok {
			return sample{}, false
		}
		name, labels, rest = line[:open], parsed, line[end+1:]
	} else {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return sample{}, false
		}
		name, rest = fields[0], strings.Join(fields[1:], " ")
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return sample{}, false
	}
	value, err := parseValue(fields[0])
	if err != nil {
		return sample{}, false
	}
	return sample{name: name, labels: labels, value: value}, true
}

func parseValue(s string) (float64, error) {
	switch s {
	case "+Inf", "Inf":
		return math.Inf(1), nil
	case "-Inf":
		return math.Inf(-1), nil
	}
	return strconv.ParseFloat(s, 64)
}

func parseExposition(r io.Reader) []sample {
	var out []sample
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if s, ok := parseLine(scanner.Text()); ok {
			out = append(out, s)
		}
	}
	return out
}

func matches(s sample, name string, want map[string]string) bool {
	if s.name != name {
		return false
	}
	for k, v := range want {
		if s.labels[k] != v {
			return false
		}
	}
	return true
}

func sumOf(samples []sample, name string, want map[string]string) float64 {
	total := 0.0
	for _, s := range samples {
		if matches(s, name, want) {
			total += s.value
		}
	}
	return total
}

func buckets(samples []sample, name string, want map[string]string) map[float64]float64 {
	out := map[float64]float64{}
	for _, s := range samples {
		if !matches(s, name+"_bucket", want) {
			continue
		}
		le, err := parseValue(s.labels["le"])
		if err != nil {
			continue
		}
		out[le] += s.value
	}
	return out
}

func bucketDelta(before, after map[float64]float64) map[float64]float64 {
	out := map[float64]float64{}
	for le, v := range after {
		out[le] = v - before[le]
	}
	return out
}

func addBuckets(parts ...map[float64]float64) map[float64]float64 {
	out := map[float64]float64{}
	for _, p := range parts {
		for le, v := range p {
			out[le] += v
		}
	}
	return out
}

func histogramQuantile(q float64, delta map[float64]float64) float64 {
	if len(delta) == 0 {
		return math.NaN()
	}
	bounds := make([]float64, 0, len(delta))
	for le := range delta {
		bounds = append(bounds, le)
	}
	sort.Float64s(bounds)
	total := delta[bounds[len(bounds)-1]]
	if total <= 0 {
		return math.NaN()
	}
	rank := q * total
	prevBound, prevCount := 0.0, 0.0
	for _, le := range bounds {
		count := delta[le]
		if count >= rank {
			if math.IsInf(le, 1) {
				return prevBound
			}
			if count == prevCount {
				return le
			}
			return prevBound + (le-prevBound)*(rank-prevCount)/(count-prevCount)
		}
		prevBound, prevCount = le, count
	}
	return prevBound
}

type scraper struct {
	client  *http.Client
	baseURL string
}

func (s scraper) scrape(ctx context.Context) ([]sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape %s: %w", s.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scrape %s: status %d", s.baseURL, resp.StatusCode)
	}
	return parseExposition(resp.Body), nil
}

type gaugeMax struct {
	mu   sync.Mutex
	maxs map[string]float64
}

func newGaugeMax() *gaugeMax {
	return &gaugeMax{maxs: map[string]float64{}}
}

func (g *gaugeMax) observe(key string, v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if cur, ok := g.maxs[key]; !ok || v > cur {
		g.maxs[key] = v
	}
}

func (g *gaugeMax) get(key string) float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.maxs[key]
}

type gaugeQuery struct {
	key    string
	name   string
	labels map[string]string
}

func trackGauges(ctx context.Context, scrapers []scraper, queries []gaugeQuery, every time.Duration) (*gaugeMax, func()) {
	tracker := newGaugeMax()
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	sampleOnce := func() {
		for i, sc := range scrapers {
			samples, err := sc.scrape(ctx)
			if err != nil {
				continue
			}
			for _, q := range queries {
				tracker.observe(fmt.Sprintf("%s#%d", q.key, i), sumOf(samples, q.name, q.labels))
			}
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		sampleOnce()
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sampleOnce()
			}
		}
	}()
	return tracker, func() {
		cancel()
		wg.Wait()
	}
}

func histogramDelta(before, after [][]sample, name string, want map[string]string) map[float64]float64 {
	var parts []map[float64]float64
	for i := range after {
		if i >= len(before) || before[i] == nil || after[i] == nil {
			continue
		}
		parts = append(parts, bucketDelta(buckets(before[i], name, want), buckets(after[i], name, want)))
	}
	return addBuckets(parts...)
}

func counterDelta(before, after [][]sample, name string, want map[string]string) float64 {
	total := 0.0
	for i := range after {
		if i >= len(before) || before[i] == nil || after[i] == nil {
			continue
		}
		total += sumOf(after[i], name, want) - sumOf(before[i], name, want)
	}
	return total
}
