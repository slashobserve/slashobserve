package revisions_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestASecondPersonsRunIsClaimedBeforeTheFirstPersonsBacklog(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	backlog := []string{observeFor(t, svc, user, "first"), observeFor(t, svc, user, "second"), observeFor(t, svc, user, "third")}
	single := observeFor(t, svc, eve, "eve's only question")

	var claimed []string
	for range 4 {
		row, found := claimOne(t, svc)
		if !found {
			t.Fatalf("claimed only %v", claimed)
		}
		claimed = append(claimed, row.ID.String())
	}
	if want := []string{backlog[0], single, backlog[1], backlog[2]}; !slices.Equal(claimed, want) {
		t.Fatalf("claimed %v, want the first backlog run, then Eve's, then the rest of the backlog %v", claimed, want)
	}
}
