package tools

const (
	PhaseDiscover = "discover"
	PhaseLint     = "lint"
	PhaseQuery    = "query"
	PhaseSave     = "save"
)

func Phase(name string) string {
	switch name {
	case nameLintQuery:
		return PhaseLint
	case nameQueryRange, nameQueryInstant:
		return PhaseQuery
	case nameCreateDashboard, nameUpdateDashboard, nameUpdatePanels, nameImportGrafana, nameSaveFinding:
		return PhaseSave
	}
	return PhaseDiscover
}
