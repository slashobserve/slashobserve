package threads

import (
	"slices"
	"testing"
)

func TestThreadMachineIsConsistent(t *testing.T) {
	if err := ThreadMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestAllowedActionsFollowTheStatusAndWhatTheCallerMayDo(t *testing.T) {
	tests := []struct {
		name   string
		status string
		rights rights
		want   []string
	}{
		{"the starter of an open thread", StatusOpen, rights{Comment: true, Started: true}, []string{"reply", "resolve"}},
		{"a commenter who did not start an open thread", StatusOpen, rights{Comment: true}, []string{"reply"}},
		{"an editor who did not start an open thread", StatusOpen, rights{Comment: true, Edit: true}, []string{"reply", "resolve"}},
		{"a commenter on a resolved thread they did not start", StatusResolved, rights{Comment: true}, []string{"reply", "reopen"}},
		{"a viewer who started an open thread", StatusOpen, rights{Started: true}, []string{}},
		{"open without comment", StatusOpen, rights{}, []string{}},
		{"resolved without comment", StatusResolved, rights{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allowedActions(tt.status, tt.rights)
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("allowedActions(%s, %+v) = %#v, want %#v", tt.status, tt.rights, got, tt.want)
			}
		})
	}
}

func TestThreadMachineOffersEveryActionAfterCreation(t *testing.T) {
	var got []string
	for _, a := range ThreadMachine.Offered() {
		got = append(got, string(a))
	}
	if want := []string{"reply", "resolve", "reopen"}; !slices.Equal(got, want) {
		t.Errorf("Offered = %v, want %v", got, want)
	}
}
