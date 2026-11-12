package guard

import (
	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

type State struct {
	Mode          tools.Mode      `json:"mode"`
	ParentVersion int             `json:"parent_version"`
	Building      bool            `json:"building"`
	Rejections    int             `json:"rejections"`
	Window        Window          `json:"window"`
	LastGuard     string          `json:"last_guard,omitempty"`
	Churn         map[string]int  `json:"churn,omitempty"`
	ChurnedInRow  int             `json:"churned_in_row,omitempty"`
	Reads         map[string]Read `json:"reads,omitempty"`
	Saves         map[string]bool `json:"saves,omitempty"`
	Turn          Turn            `json:"turn"`
	Counts        Counts          `json:"counts"`
}

type Window struct {
	Calls    int  `json:"calls"`
	Stalled  int  `json:"stalled"`
	Diagnose bool `json:"diagnose,omitempty"`
}

type Read struct {
	Times      int  `json:"times"`
	Replayable bool `json:"replayable,omitempty"`
}

type Turn struct {
	ReadGuard string             `json:"read_guard,omitempty"`
	SaveSeen  bool               `json:"save_seen,omitempty"`
	Answered  int                `json:"answered"`
	Progress  bool               `json:"progress,omitempty"`
	Rejected  bool               `json:"rejected,omitempty"`
	Keys      map[string]TurnKey `json:"keys,omitempty"`
}

type TurnKey struct {
	Position int  `json:"position"`
	Refused  bool `json:"refused,omitempty"`
}

type Counts struct {
	ToolCalls        int `json:"tool_calls"`
	Repairs          int `json:"repairs"`
	BuildSaves       int `json:"build_saves"`
	Nudges           int `json:"nudges"`
	SaveNowRefusals  int `json:"save_now_refusals"`
	StalledTurns     int `json:"stalled_turns"`
	OnePanelRefusals int `json:"one_panel_refusals"`
}

func New(mode tools.Mode) State {
	return State{Mode: mode}
}

func (s *State) BeginSession(building bool, parentVersion int) {
	s.ParentVersion = parentVersion
	s.Reads = nil
	s.Window = Window{}
	s.Building = building
}

func (s *State) Restart() {
	s.Rejections = 0
	s.Window = Window{}
	s.Churn, s.ChurnedInRow = nil, 0
	s.Reads, s.Saves = nil, nil
}

func (s *State) BeginTurn(hasSave bool) {
	s.Window.Calls++
	s.Turn = Turn{ReadGuard: readRefusal(s.Mode, s.Window, hasSave)}
}
