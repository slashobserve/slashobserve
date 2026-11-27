package folders

import (
	"slices"
	"testing"
)

func TestADashboardHasOneOriginAndAStarterWinsOverAFile(t *testing.T) {
	cases := []struct {
		name   string
		access DashboardAccess
		want   string
	}{
		{"made in the app", DashboardAccess{}, "editable"},
		{"shipped by the server", DashboardAccess{Provisioned: true}, "starter"},
		{"bound to a provisioning file", DashboardAccess{FromFile: true}, "file"},
		{"written by observe apply", DashboardAccess{Applied: true}, "applied"},
		{"a starter that also has a file", DashboardAccess{Provisioned: true, FromFile: true, Applied: true}, "starter"},
		{"a provisioning file that was once applied", DashboardAccess{FromFile: true, Applied: true}, "file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.access.Origin().String(); got != tc.want {
				t.Fatalf("origin = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEveryOriginIsADeclaredState(t *testing.T) {
	origins := []DashboardOrigin{OriginStarter, OriginFile, OriginApplied, OriginEditable}
	var named []string
	for _, o := range origins {
		named = append(named, o.String())
	}
	if declared := DashboardOrigins.Names(); !slices.Equal(named, declared) {
		t.Fatalf("origins = %v, declared states = %v", named, declared)
	}
}

func TestOnlyAStarterRefusesAChangeToItsContent(t *testing.T) {
	for _, access := range []DashboardAccess{{FromFile: true}, {Applied: true}, {}} {
		access.Abilities = Abilities{View: true, Comment: true, Edit: true}
		if access.Refuses(NeedContent) {
			t.Errorf("a dashboard with origin %s refuses an edit, want it to save", access.Origin())
		}
	}
	starter := DashboardAccess{Provisioned: true, Abilities: Abilities{View: true, Comment: true, Edit: true}}
	if !starter.Refuses(NeedContent) {
		t.Error("a starter accepts a change to its content")
	}
}
