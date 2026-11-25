package observations

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func TestFieldsAreShownOnlyForAnswersTheViewerSees(t *testing.T) {
	viewer := actor.Actor{Kind: auth.KindUser, ID: "viewer"}
	other := actor.Actor{Kind: auth.KindUser, ID: "other"}
	fields := []RunFields{
		{RunID: "live", Fields: findings.Structured{Impact: "a"}},
		{RunID: "mine", Fields: findings.Structured{Impact: "b"}},
		{RunID: "theirs", Fields: findings.Structured{Impact: "c"}},
		{RunID: "panel", Fields: findings.Structured{Impact: "d"}},
		{RunID: "gone", Fields: findings.Structured{Impact: "e"}},
	}
	shown := []Block{
		{Kind: KindText, State: BlockLive, RunID: "live", Actor: other},
		{Kind: KindText, State: BlockRequested, RunID: "mine", Actor: viewer},
		{Kind: KindText, State: BlockRequested, RunID: "theirs", Actor: other},
		{Kind: KindPanel, State: BlockLive, RunID: "panel", Actor: other},
	}
	var got []string
	for _, f := range fieldsOfShownAnswers(fields, shown, viewer) {
		got = append(got, f.RunID)
	}
	if want := []string{"live", "mine"}; !slices.Equal(got, want) {
		t.Errorf("fields shown for runs %v, want %v", got, want)
	}
}

func TestNoFieldsAreShownWhenNoAnswerIsVisible(t *testing.T) {
	fields := []RunFields{{RunID: "r1", Fields: findings.Structured{NoIncident: true}}}
	if got := fieldsOfShownAnswers(fields, nil, actor.Actor{}); len(got) != 0 {
		t.Errorf("fields = %v, want none", got)
	}
}
