package threads

import (
	"strings"
	"testing"
)

func (r *rig) closedProposalBodies(t *testing.T, tg Target) []string {
	t.Helper()
	page := r.list(t, testUser, tg)
	bodies := make([]string, len(page.Messages))
	for i, m := range page.Messages {
		bodies[i] = m.Body
	}
	return bodies
}

func TestDiscardingAProposalPostsTheReasonInItsDiscussion(t *testing.T) {
	r := newRig(t)
	r.proposals.OnClosed(r.service)
	id := r.createProposal(t, r.createDashboard(t))
	tg := target(KindProposal, id)

	if _, err := r.proposals.Discard(t.Context(), testUser, id, "the panel is already there"); err != nil {
		t.Fatal(err)
	}
	bodies := r.closedProposalBodies(t, tg)
	if len(bodies) != 1 || bodies[0] != "Discarded.\n\nReason: the panel is already there" {
		t.Fatalf("discussion = %q, want the discard and its reason", bodies)
	}
	if page := r.list(t, testUser, tg); page.Messages[0].Author.ID != testUser.ID {
		t.Fatalf("author = %+v, want the person who discarded it", page.Messages[0].Author)
	}
}

func TestDiscardingAProposalWithoutAReasonPostsOnlyTheDiscard(t *testing.T) {
	r := newRig(t)
	r.proposals.OnClosed(r.service)
	id := r.createProposal(t, r.createDashboard(t))

	if _, err := r.proposals.Discard(t.Context(), testUser, id, ""); err != nil {
		t.Fatal(err)
	}
	if bodies := r.closedProposalBodies(t, target(KindProposal, id)); len(bodies) != 1 || bodies[0] != "Discarded." {
		t.Fatalf("discussion = %q, want only the discard", bodies)
	}
}

func TestADiscardReasonThatCannotBePostedStillPostsTheDiscard(t *testing.T) {
	r := newRig(t)
	r.proposals.OnClosed(r.service)
	id := r.createProposal(t, r.createDashboard(t))

	reason := "ask @[Nobody](user:00000000-0000-4000-8000-00000000dead)"
	if _, err := r.proposals.Discard(t.Context(), testUser, id, reason); err != nil {
		t.Fatal(err)
	}
	if bodies := r.closedProposalBodies(t, target(KindProposal, id)); len(bodies) != 1 || bodies[0] != "Discarded." {
		t.Fatalf("discussion = %q, want the discard without the reason", bodies)
	}
}

func TestMergingAProposalPostsTheVersionItSavedInItsDiscussion(t *testing.T) {
	r := newRig(t)
	r.proposals.OnClosed(r.service)
	id := r.createProposal(t, r.createDashboard(t))

	if _, err := r.proposals.Merge(t.Context(), testUser, id, 1, nil); err != nil {
		t.Fatal(err)
	}
	bodies := r.closedProposalBodies(t, target(KindProposal, id))
	if len(bodies) != 1 || !strings.HasPrefix(bodies[0], "Merged as version ") {
		t.Fatalf("discussion = %q, want the merge and its version", bodies)
	}
}

func TestAProposalClosesWithoutADiscussionPostWhenNothingListens(t *testing.T) {
	r := newRig(t)
	id := r.createProposal(t, r.createDashboard(t))

	if _, err := r.proposals.Discard(t.Context(), testUser, id, "no"); err != nil {
		t.Fatal(err)
	}
	if bodies := r.closedProposalBodies(t, target(KindProposal, id)); len(bodies) != 0 {
		t.Fatalf("discussion = %q, want nothing", bodies)
	}
}
