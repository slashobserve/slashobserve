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

func TestAllowedActionsFollowTheStatusAndTheCommentAbility(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		canComment bool
		want       []string
	}{
		{"open with comment", StatusOpen, true, []string{"reply", "resolve"}},
		{"resolved with comment", StatusResolved, true, []string{"reply", "reopen"}},
		{"open without comment", StatusOpen, false, []string{}},
		{"resolved without comment", StatusResolved, false, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allowedActions(tt.status, tt.canComment)
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("allowedActions(%s, %v) = %#v, want %#v", tt.status, tt.canComment, got, tt.want)
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
