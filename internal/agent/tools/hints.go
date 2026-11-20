package tools

var destructiveTools = map[string]bool{
	nameUpdatePanels:            true,
	nameUpdateDashboard:         true,
	nameRestoreDashboardVersion: true,
	nameUpdateAnnotation:        true,
	nameUpdateBlock:             true,
	nameUpdateObservation:       true,
	nameUpdateSavedSearch:       true,
	nameEditComment:             true,
	nameRenameFolder:            true,
	nameDiscardBlock:            true,
	nameDiscardProposal:         true,
	nameMergeProposal:           true,
	nameSetHomeDashboard:        true,
	nameClearHomeDashboard:      true,
	nameCancelRun:               true,
	nameSetInboxItemDone:        true,
}
