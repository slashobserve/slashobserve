package main

import (
	"math"
	"slices"
	"sort"
	"sync"
	"time"
)

const tooManyRequests = 429

type recorder struct {
	mu        sync.Mutex
	durations []time.Duration
	statuses  map[int]int
	codes     map[string]int
}

func newRecorder() *recorder {
	return &recorder{statuses: map[int]int{}, codes: map[string]int{}}
}

func (r *recorder) Observe(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durations = append(r.durations, d)
}

func (r *recorder) Fail(status int, code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses[status]++
	if code != "" {
		r.codes[code]++
	}
}

func (r *recorder) Quantile(q float64) time.Duration {
	r.mu.Lock()
	sorted := append([]time.Duration(nil), r.durations...)
	r.mu.Unlock()
	if len(sorted) == 0 {
		return 0
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

func (r *recorder) Max() time.Duration {
	return r.Quantile(1)
}

func (r *recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.durations)
}

func (r *recorder) Failures() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, n := range r.statuses {
		total += n
	}
	return total
}

func (r *recorder) Total() int {
	return r.Count() + r.Failures()
}

func (r *recorder) Statuses() map[int]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[int]int, len(r.statuses))
	for status, n := range r.statuses {
		out[status] = n
	}
	return out
}

func (r *recorder) Codes() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.codes))
	for code, n := range r.codes {
		out[code] = n
	}
	return out
}

func (r *recorder) ServerErrors() int {
	total := 0
	for status, n := range r.Statuses() {
		if status == 0 || status >= 500 {
			total += n
		}
	}
	return total
}

func (r *recorder) TooMany() int {
	return r.Statuses()[tooManyRequests]
}

func (r *recorder) OtherClientErrors() int {
	total := 0
	for status, n := range r.Statuses() {
		if status >= 400 && status < 500 && status != tooManyRequests {
			total += n
		}
	}
	return total
}

func merge(recorders ...*recorder) *recorder {
	out := newRecorder()
	for _, r := range recorders {
		r.mu.Lock()
		out.durations = append(out.durations, r.durations...)
		for status, n := range r.statuses {
			out.statuses[status] += n
		}
		for code, n := range r.codes {
			out.codes[code] += n
		}
		r.mu.Unlock()
	}
	return out
}

const maxErrorMessages = 3

type errorSample struct {
	Tool     string   `json:"tool"`
	Code     string   `json:"code"`
	Count    int      `json:"count"`
	Messages []string `json:"messages"`
}

type errorLog struct {
	mu      sync.Mutex
	samples map[[2]string]*errorSample
}

func newErrorLog() *errorLog {
	return &errorLog{samples: map[[2]string]*errorSample{}}
}

func (l *errorLog) Add(tool, code, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := [2]string{tool, code}
	s := l.samples[key]
	if s == nil {
		s = &errorSample{Tool: tool, Code: code}
		l.samples[key] = s
	}
	s.Count++
	if message != "" && len(s.Messages) < maxErrorMessages && !slices.Contains(s.Messages, message) {
		s.Messages = append(s.Messages, message)
	}
}

func (l *errorLog) Samples() []errorSample {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]errorSample, 0, len(l.samples))
	for _, s := range l.samples {
		out = append(out, errorSample{Tool: s.Tool, Code: s.Code, Count: s.Count, Messages: slices.Clone(s.Messages)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		return out[i].Code < out[j].Code
	})
	return out
}
