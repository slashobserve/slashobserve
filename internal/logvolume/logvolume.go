package logvolume

import (
	"slices"
	"strings"
	"time"
)

type Kind string

const (
	Error   Kind = "error"
	Warning Kind = "warning"
	Info    Kind = "info"
	Other   Kind = "other"
)

var Kinds = []Kind{Error, Warning, Info, Other}

var aliases = map[Kind][]string{
	Error:   {"err", "eror", "error", "emerg", "fatal", "alert", "crit", "critical", "panic"},
	Warning: {"warn", "warning"},
	Info:    {"info", "information", "informational", "notice"},
}

var kindOfWord = func() map[string]Kind {
	m := map[string]Kind{}
	for k, words := range aliases {
		for _, w := range words {
			m[w] = k
		}
	}
	return m
}()

func Aliases(k Kind) []string {
	return slices.Clone(aliases[k])
}

func KindOf(word string) Kind {
	if k, ok := kindOfWord[strings.ToLower(strings.TrimSpace(word))]; ok {
		return k
	}
	return Other
}

type Bucket struct {
	Start                       time.Time
	Error, Warning, Info, Other int64
}

func (b *Bucket) add(k Kind, n int64) {
	switch k {
	case Error:
		b.Error += n
	case Warning:
		b.Warning += n
	case Info:
		b.Info += n
	default:
		b.Other += n
	}
}

func (b Bucket) total() int64 {
	return b.Error + b.Warning + b.Info + b.Other
}

type Volume struct {
	StepMS  int64
	Total   int64
	Buckets []Bucket
}

type Histogram struct {
	from    time.Time
	step    time.Duration
	buckets []Bucket
}

func New(from time.Time, step time.Duration, buckets int) *Histogram {
	h := &Histogram{from: from, step: step, buckets: make([]Bucket, max(buckets, 0))}
	for i := range h.buckets {
		h.buckets[i].Start = from.Add(time.Duration(i) * step)
	}
	return h
}

func (h *Histogram) Add(start time.Time, k Kind, n int64) {
	if start.Before(h.from) {
		return
	}
	i := int64(start.Sub(h.from) / h.step)
	if i >= int64(len(h.buckets)) {
		return
	}
	h.buckets[i].add(k, n)
}

func (h *Histogram) Volume() Volume {
	v := Volume{StepMS: h.step.Milliseconds(), Buckets: slices.Clone(h.buckets)}
	for _, b := range v.Buckets {
		v.Total += b.total()
	}
	return v
}
