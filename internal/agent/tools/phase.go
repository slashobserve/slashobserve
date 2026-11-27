package tools

import "sync"

const (
	PhaseDiscover = "discover"
	PhaseLint     = "lint"
	PhaseQuery    = "query"
	PhaseSave     = "save"
	PhaseWrite    = "write"
	PhaseLedger   = "ledger"
)

var phases = sync.OnceValue(func() map[string]string {
	table := map[string]string{}
	for _, t := range New(Deps{LedgerTool: true}).List() {
		table[t.Name] = phaseOf(t)
	}
	return table
})

func phaseOf(t Tool) string {
	switch {
	case saveTools[t.Name] || t.Answers:
		return PhaseSave
	case t.Name == nameNoteHypotheses:
		return PhaseLedger
	case t.Name == nameLintQuery:
		return PhaseLint
	case t.Name == nameQueryRange || t.Name == nameQueryInstant || t.Name == nameMeasure:
		return PhaseQuery
	case t.ReadOnly:
		return PhaseDiscover
	}
	return PhaseWrite
}

var saveTools = map[string]bool{
	nameCreateDashboard: true,
	nameUpdateDashboard: true,
	nameUpdatePanels:    true,
	nameImportGrafana:   true,
	nameSaveFinding:     true,
}

func Phase(name string) string {
	if phase, ok := phases()[name]; ok {
		return phase
	}
	return PhaseDiscover
}
