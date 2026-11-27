package triage

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

const (
	EventLookback = 30 * time.Minute
	EventSlack    = 5 * time.Minute

	maxCandidateOrigins = 3
	maxCandidateEvents  = 5
	maxCandidatePattern = 3
)

type move struct {
	service string
	rows    []Row
	onset   *time.Time
}

func Cluster(f Findings) ([]Candidate, []Row) {
	moves := movesOf(f.Rows)
	var candidates []Candidate
	root := map[string]int{}
	for _, m := range moves {
		if i, ok := upstreamOf(m, candidates, root, f.Origins); ok {
			candidates[i].KnockOn = append(candidates[i].KnockOn, m.service)
			root[m.service] = i
			continue
		}
		root[m.service] = len(candidates)
		candidates = append(candidates, Candidate{Service: m.service, Onset: m.onset, Rows: m.rows})
	}
	for i := range candidates {
		c := &candidates[i]
		c.ID = fmt.Sprintf("c%d", i+1)
		c.Origins = originsOf(c.Service, f.Origins)
		c.Events = eventsNear(c, f.Events)
		c.Patterns = patternsOf(c.Service, f.Patterns)
	}
	return candidates, backgroundOf(f.Rows, root)
}

func movesOf(rows []Row) []move {
	byService := map[string]*move{}
	var order []string
	for _, r := range rows {
		if !r.Moved || (r.Signal != SignalErrors && r.Signal != SignalLatency) {
			continue
		}
		m, ok := byService[r.Service]
		if !ok {
			m = &move{service: r.Service}
			byService[r.Service] = m
			order = append(order, r.Service)
		}
		m.keepFinest(r)
	}
	moves := make([]move, 0, len(order))
	for _, name := range order {
		m := byService[name]
		m.settle()
		moves = append(moves, *m)
	}
	slices.SortStableFunc(moves, byOnset)
	return moves
}

func (m *move) keepFinest(r Row) {
	if i := slices.IndexFunc(m.rows, func(x Row) bool { return x.Signal == r.Signal }); i >= 0 {
		if m.rows[i].Rung <= r.Rung {
			return
		}
		m.rows = slices.Delete(m.rows, i, i+1)
	}
	m.rows = append(m.rows, r)
}

func (m *move) settle() {
	slices.SortFunc(m.rows, func(a, b Row) int { return cmp.Compare(a.Signal, b.Signal) })
	for _, r := range m.rows {
		if r.Onset != nil && (m.onset == nil || r.Onset.Before(*m.onset)) {
			m.onset = r.Onset
		}
	}
}

func byOnset(a, b move) int {
	switch {
	case a.onset == nil && b.onset == nil:
		return cmp.Compare(a.service, b.service)
	case a.onset == nil:
		return 1
	case b.onset == nil:
		return -1
	}
	return cmp.Or(a.onset.Compare(*b.onset), cmp.Compare(a.service, b.service))
}

func upstreamOf(m move, candidates []Candidate, root map[string]int, origins []Origin) (int, bool) {
	for _, o := range origins {
		if o.Service == m.service || !slices.Contains(o.RelayedBy, m.service) {
			continue
		}
		i, ok := root[o.Service]
		if !ok {
			continue
		}
		up := candidates[i]
		if m.onset != nil && up.Onset != nil && m.onset.Before(*up.Onset) {
			continue
		}
		return i, true
	}
	return 0, false
}

func originsOf(service string, origins []Origin) []Origin {
	var out []Origin
	for _, o := range origins {
		if o.Service == service {
			out = append(out, o)
		}
	}
	slices.SortStableFunc(out, func(a, b Origin) int { return cmp.Compare(b.Traces, a.Traces) })
	return out[:min(len(out), maxCandidateOrigins)]
}

func eventsNear(c *Candidate, events []Event) []Event {
	if c.Onset == nil {
		return nil
	}
	from, to := c.Onset.Add(-EventLookback), c.Onset.Add(EventSlack)
	var out []Event
	for _, e := range events {
		if e.At.Before(from) || e.At.After(to) {
			continue
		}
		if e.Service == "" || e.Service == c.Service || slices.Contains(c.KnockOn, e.Service) {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Event) int { return b.At.Compare(a.At) })
	return out[:min(len(out), maxCandidateEvents)]
}

func patternsOf(service string, patterns []Pattern) []Pattern {
	var out []Pattern
	for _, p := range patterns {
		if p.Service == service {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b Pattern) int {
		return cmp.Compare(b.Window-b.Baseline, a.Window-a.Baseline)
	})
	return out[:min(len(out), maxCandidatePattern)]
}

func backgroundOf(rows []Row, candidates map[string]int) []Row {
	var out []Row
	for _, r := range rows {
		if r.Moved || r.Signal != SignalErrors || r.Window <= 0 || r.Baseline <= 0 {
			continue
		}
		if _, ok := candidates[r.Service]; ok {
			continue
		}
		if i := slices.IndexFunc(out, func(x Row) bool { return x.Service == r.Service }); i >= 0 {
			if out[i].Rung > r.Rung {
				out[i] = r
			}
			continue
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Row) int { return cmp.Compare(a.Service, b.Service) })
	return out
}
