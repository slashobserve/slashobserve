package folders

import "github.com/slashobserve/slashobserve/internal/lifecycle"

type ManagedBy int

const (
	ManagedByWorkspace ManagedBy = iota
	ManagedByStarter
	ManagedByFile
	ManagedByApply
)

const (
	managedByStarter   = "starter"
	managedByFile      = "file"
	managedByApply     = "apply"
	managedByWorkspace = "workspace"
)

const (
	fileEditWarning  = "This dashboard comes from a file in the server's provisioning directory. A change to that file replaces this edit, which stays in the version history. Change the file to keep the edit."
	applyEditWarning = "This dashboard was last written by observe dashboards apply from a file. Applying that file again after it changes replaces this edit, which stays in the version history. Change the file to keep the edit."
)

var ManagedByStates = lifecycle.Derived{Name: "dashboard managed by", States: []lifecycle.DerivedState{
	{Name: managedByStarter, When: "The server ships the dashboard and keeps it current with each release. Every change to its content answers 409 `provisioned`; duplicate it to change a copy."},
	{Name: managedByFile, When: "A file in the server's provisioning directory is bound to the dashboard. It can be edited, and a change to that file replaces the edit, which stays in the version history. The dashboard keeps this value after the file is removed or provisioning is turned off."},
	{Name: managedByApply, When: "`observe dashboards apply` last bound a file to the dashboard. It can be edited, and applying that file again after it changes replaces the edit, which stays in the version history."},
	{Name: managedByWorkspace, When: "None of the above: people edit the dashboard in the app, and no file and no release writes it."},
}}

func (m ManagedBy) String() string {
	switch m {
	case ManagedByStarter:
		return managedByStarter
	case ManagedByFile:
		return managedByFile
	case ManagedByApply:
		return managedByApply
	}
	return managedByWorkspace
}

func (a DashboardAccess) ManagedBy() ManagedBy {
	switch {
	case a.Provisioned:
		return ManagedByStarter
	case a.FromFile:
		return ManagedByFile
	case a.Applied:
		return ManagedByApply
	}
	return ManagedByWorkspace
}

func (m ManagedBy) EditWarning() string {
	switch m {
	case ManagedByFile:
		return fileEditWarning
	case ManagedByApply:
		return applyEditWarning
	default:
		return ""
	}
}
