package triage

import (
	"time"
)

const (
	SignalRequests = "requests"
	SignalErrors   = "errors"
	SignalLatency  = "latency"

	EventAnnotation = "annotation"
	EventDeployLine = "deploy lines"
	EventFlag       = "flag"
)

var Rungs = []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour}

type Row struct {
	Service  string
	Signal   string
	Rung     time.Duration
	Window   float64
	Baseline float64
	Unit     string
	Source   string
	Basis    string
	Moved    bool
	Onset    *time.Time
}

type Origin struct {
	Source    string
	Service   string
	Operation string
	Message   string
	Traces    int
	Share     float64
	Example   string
	RelayedBy []string
}

type Event struct {
	Kind    string
	Source  string
	Service string
	At      time.Time
	Text    string
}

type Pattern struct {
	Source   string
	Service  string
	Level    string
	Text     string
	Window   int64
	Baseline int64
}

func (p Pattern) New() bool { return p.Baseline == 0 }

type ServiceSource struct {
	Source   string
	Type     string
	Signal   string
	Field    string
	Services []string
}

type Skip struct {
	Step   string
	Source string
	Reason string
}

type Findings struct {
	Rows     []Row
	Origins  []Origin
	Events   []Event
	Patterns []Pattern
}

type Candidate struct {
	ID       string
	Service  string
	Onset    *time.Time
	Rows     []Row
	KnockOn  []string
	Origins  []Origin
	Events   []Event
	Patterns []Pattern
}

type Report struct {
	End        time.Time
	Question   *Window
	Services   []ServiceSource
	Signals    []string
	Rates      []Row
	Candidates []Candidate
	Background []Row
	Skipped    []Skip
	Calls      int
	Duration   time.Duration
}

type Window struct{ From, To time.Time }
