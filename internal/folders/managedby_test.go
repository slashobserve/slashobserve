package folders

import (
	"slices"
	"strings"
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

func TestAnEditToAFileOrAppliedDashboardWarnsWithoutPromisingAReplacement(t *testing.T) {
	for _, m := range []ManagedBy{ManagedByWorkspace, ManagedByStarter} {
		if got := m.EditWarning(); got != "" {
			t.Errorf("an edit to a dashboard managed by %s warns %q, want no warning", m, got)
		}
	}
	file := ManagedByFile.EditWarning()
	if !strings.Contains(file, "provisioning directory") || !strings.Contains(file, "A change to that file replaces this edit") {
		t.Errorf("the file warning = %q, want it to say that a change to the provisioning file replaces the edit", file)
	}
	apply := ManagedByApply.EditWarning()
	if !strings.Contains(apply, "observe dashboards apply") || !strings.Contains(apply, "replaces this edit") {
		t.Errorf("the apply warning = %q, want it to say that applying the changed file again replaces the edit", apply)
	}
	for _, warning := range []string{file, apply} {
		if strings.Contains(warning, "The next") {
			t.Errorf("warning %q promises a replacement that may never come", warning)
		}
		if !strings.Contains(warning, "stays in the version history") {
			t.Errorf("warning %q does not say where the replaced edit stays", warning)
		}
	}
}
