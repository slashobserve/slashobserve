package folders

import (
	"slices"
	"testing"
)

func TestADashboardIsManagedByOneThingAndAStarterWinsOverAFile(t *testing.T) {
	cases := []struct {
		name   string
		access DashboardAccess
		want   string
	}{
		{"made in the app", DashboardAccess{}, "workspace"},
		{"shipped by the server", DashboardAccess{Provisioned: true}, "starter"},
		{"bound to a provisioning file", DashboardAccess{FromFile: true}, "file"},
		{"written by observe apply", DashboardAccess{Applied: true}, "apply"},
		{"a starter that also has a file", DashboardAccess{Provisioned: true, FromFile: true, Applied: true}, "starter"},
		{"a provisioning file that was once applied", DashboardAccess{FromFile: true, Applied: true}, "file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.access.ManagedBy().String(); got != tc.want {
				t.Fatalf("managed_by = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEveryManagedByValueIsADeclaredState(t *testing.T) {
	values := []ManagedBy{ManagedByStarter, ManagedByFile, ManagedByApply, ManagedByWorkspace}
	var named []string
	for _, m := range values {
		named = append(named, m.String())
	}
	if declared := ManagedByStates.Names(); !slices.Equal(named, declared) {
		t.Fatalf("managed_by values = %v, declared states = %v", named, declared)
	}
}

func TestOnlyAStarterRefusesAChangeToItsContent(t *testing.T) {
	for _, access := range []DashboardAccess{{FromFile: true}, {Applied: true}, {}} {
		access.Abilities = Abilities{View: true, Comment: true, Edit: true}
		if access.Refuses(NeedContent) {
			t.Errorf("a dashboard managed by %s refuses an edit, want it to save", access.ManagedBy())
		}
	}
	starter := DashboardAccess{Provisioned: true, Abilities: Abilities{View: true, Comment: true, Edit: true}}
	if !starter.Refuses(NeedContent) {
		t.Error("a starter accepts a change to its content")
	}
}
