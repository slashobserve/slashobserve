package guard

import (
	"encoding/json"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const maxUnchecked = 8

func (s *State) noteChecked(c Call, res tools.Result) {
	if tools.Phase(c.Name) == tools.PhaseLint {
		return
	}
	var in struct {
		Datasource string `json:"datasource"`
	}
	if json.Unmarshal(c.Input, &in) != nil || in.Datasource == "" {
		return
	}
	same := func(u Unchecked) bool { return u.Tool == c.Name && u.Datasource == in.Datasource }
	switch {
	case res.Unchecked != "":
		if i := slices.IndexFunc(s.Unchecked, same); i >= 0 {
			s.Unchecked[i].Code = res.Unchecked
		} else if len(s.Unchecked) < maxUnchecked {
			s.Unchecked = append(s.Unchecked, Unchecked{Tool: c.Name, Datasource: in.Datasource, Code: res.Unchecked})
		}
	case !res.IsError:
		s.Unchecked = slices.DeleteFunc(s.Unchecked, same)
	}
}

func (s *State) uncheckedReads() []tools.Unchecked {
	var out []tools.Unchecked
	for _, u := range s.Unchecked {
		item := tools.Unchecked{Datasource: u.Datasource, Code: u.Code}
		if !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}
