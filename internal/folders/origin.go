package folders

import "github.com/slashobserve/slashobserve/internal/lifecycle"

type DashboardOrigin int

const (
	OriginEditable DashboardOrigin = iota
	OriginStarter
	OriginFile
	OriginApplied
)

const (
	originStarter  = "starter"
	originFile     = "file"
	originApplied  = "applied"
	originEditable = "editable"
)

const FileOwnedWarning = "This dashboard comes from a file in the server's provisioning directory. The next change to that file replaces this edit, which stays in the version history. Change the file to keep the edit."

var DashboardOrigins = lifecycle.Derived{Name: "dashboard origin", States: []lifecycle.DerivedState{
	{Name: originStarter, When: "The server ships the dashboard and keeps it current with each release. Every change to its content answers 409 `provisioned`; duplicate it to change a copy."},
	{Name: originFile, When: "A file in the server's provisioning directory is bound to the dashboard. It can be edited, and the next change to that file replaces the edit, which stays in the version history."},
	{Name: originApplied, When: "`observe dashboards apply` last bound a file to the dashboard. It can be edited, and applying that file again after it changed saves the file's content on top."},
	{Name: originEditable, When: "None of the above: no file and no release writes the dashboard."},
}}

func (o DashboardOrigin) String() string {
	switch o {
	case OriginStarter:
		return originStarter
	case OriginFile:
		return originFile
	case OriginApplied:
		return originApplied
	}
	return originEditable
}

func (a DashboardAccess) Origin() DashboardOrigin {
	switch {
	case a.Provisioned:
		return OriginStarter
	case a.FromFile:
		return OriginFile
	case a.Applied:
		return OriginApplied
	}
	return OriginEditable
}
