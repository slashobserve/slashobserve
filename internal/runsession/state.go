package runsession

import (
	"encoding/json"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	settledSaved     = "saved"
	settledFinding   = "finding"
	settledSuggested = "suggested"
	settledRestarted = "restarted"
	settledEnded     = "ended"

	maxStoredDigestBytes = 8 << 10
)

type State struct {
	Guard      guard.State          `json:"guard"`
	Digest     brief.Digest         `json:"digest"`
	Restarts   int                  `json:"restarts"`
	Build      *tools.BuildProgress `json:"build,omitempty"`
	Suggestion json.RawMessage      `json:"suggestion,omitempty"`
	IssueCodes []string             `json:"issue_codes,omitempty"`
	Settled    Settled              `json:"settled"`
	Usage      revisions.Usage      `json:"usage"`
	Turn       OpenTurn             `json:"turn"`
}

type Settled struct {
	Kind    string `json:"kind,omitempty"`
	Version int    `json:"version,omitempty"`
}

type OpenTurn struct {
	Seq      int    `json:"seq"`
	ID       string `json:"id,omitempty"`
	Open     bool   `json:"open,omitempty"`
	Calls    int    `json:"calls,omitempty"`
	Admitted int    `json:"admitted,omitempty"`
	Settled  int    `json:"settled,omitempty"`
}

func (s *State) addDiscovery(found tools.Discovery) {
	s.Digest.Add(found)
	kept, bytes := 0, 0
	for i := len(s.Digest.Entries) - 1; i >= 0; i-- {
		e := s.Digest.Entries[i]
		bytes += len(e.Datasource) + len(e.Of)
		for _, name := range e.Names {
			bytes += len(name)
		}
		if bytes > maxStoredDigestBytes {
			break
		}
		kept++
	}
	s.Digest.Entries = s.Digest.Entries[len(s.Digest.Entries)-kept:]
}

func (s *State) meta(restarted bool) RunMeta {
	meta := RunMeta{Build: s.Build}
	switch {
	case s.Settled.Kind == settledEnded:
		meta.Build = nil
		meta.Settled = ptr(settledEnded)
	case restarted:
		meta.Settled = ptr(settledRestarted)
	case s.Settled.Kind != "":
		meta.Settled = ptr(s.Settled.Kind)
	}
	return meta
}

func ptr[T any](v T) *T { return &v }
