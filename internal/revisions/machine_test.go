package revisions

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
)

func TestRunMachineIsSound(t *testing.T) {
	if err := RunMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestRunMachineQueriesAndEvents(t *testing.T) {
	if got := RunMachine.From(ActionCancel); !slices.Equal(got, []string{"queued", "running", "canceling"}) {
		t.Errorf("From(cancel) = %v", got)
	}
	if got := RunMachine.From(ActionClaim); !slices.Equal(got, []string{"queued", "running"}) {
		t.Errorf("From(claim) = %v", got)
	}
	if got := RunMachine.From(ActionExpire); !slices.Equal(got, []string{"queued", "running"}) {
		t.Errorf("From(expire) = %v", got)
	}
	events := []struct {
		a    Action
		to   State
		want string
	}{
		{ActionRelease, StateQueued, "run.released"},
		{ActionClaim, StateRunning, "run.started"},
		{ActionKeepBuild, StateFailed, "run.kept"},
		{ActionKeepBuild, StateCanceled, "run.kept"},
		{ActionCancel, StateCanceling, "run.canceling"},
		{ActionCancel, StateCanceled, "run.canceled"},
		{ActionCancel, StateSucceeded, "run.succeeded"},
		{ActionExhaust, StateFailed, "run.failed"},
		{ActionExpire, StateFailed, "run.failed"},
		{ActionExpire, StateSucceeded, "run.succeeded"},
		{ActionEnqueue, StateQueued, "run.queued"},
	}
	for _, e := range events {
		if got := RunMachine.EventFor(e.a, e.to); got != e.want {
			t.Errorf("EventFor(%s, %s) = %q, want %q", e.a, e.to, got, e.want)
		}
	}
}

func TestRunMachineEventsAreKnownActivityTypes(t *testing.T) {
	for state, ev := range RunMachine.Arrive {
		if !activity.Known(ev) {
			t.Errorf("arrival event %q of %s is not a known activity type", ev, state)
		}
	}
	for _, mv := range RunMachine.Moves {
		if mv.Event != "" && !activity.Known(mv.Event) {
			t.Errorf("event %q of %s is not a known activity type", mv.Event, mv.Action)
		}
	}
}

func TestRunMachineOffersOnlyWhatTheWireDocuments(t *testing.T) {
	want := []Action{ActionCancel, ActionKeepBuild, ActionContinue, ActionRetry, ActionPin, ActionArchive}
	if got := RunMachine.Offered(); !slices.Equal(got, want) {
		t.Errorf("Offered = %v, want %v", got, want)
	}
}

func TestAllowedActions(t *testing.T) {
	base := func(mod func(*runFacts)) runFacts {
		f := runFacts{state: StateFailed, source: SourcePrompt, onDashboard: true, requester: true, mayStart: true, buildPanels: 3}
		mod(&f)
		return f
	}
	cases := []struct {
		name  string
		facts runFacts
		want  []Action
	}{
		{"queued by the requester", base(func(f *runFacts) { f.state = StateQueued; f.source = SourceObserve; f.buildPanels = 0 }), []Action{ActionCancel, ActionPin, ActionArchive}},
		{"running for a non-requester who may start", base(func(f *runFacts) { f.state = StateRunning; f.requester = false }), []Action{}},
		{"canceling keeps cancel as a no-op", base(func(f *runFacts) { f.state = StateCanceling; f.buildPanels = 0 }), []Action{ActionCancel, ActionPin, ActionArchive}},
		{"running suggest takes no cancel", base(func(f *runFacts) { f.state = StateRunning; f.source = SourceSuggest; f.buildPanels = 0 }), []Action{ActionPin, ActionArchive}},
		{"failed prompt with an unsaved build", base(func(*runFacts) {}), []Action{ActionKeepBuild, ActionContinue, ActionRetry, ActionPin, ActionArchive}},
		{"the same run for a non-requester who may start", base(func(f *runFacts) { f.requester = false }), []Action{ActionRetry}},
		{"the same run for a non-requester who may not", base(func(f *runFacts) { f.requester = false; f.mayStart = false }), []Action{}},
		{"the same run for the requester who may not start", base(func(f *runFacts) { f.mayStart = false }), []Action{ActionKeepBuild, ActionContinue, ActionPin, ActionArchive}},
		{"canceled observe already continued", base(func(f *runFacts) {
			f.state = StateCanceled
			f.source = SourceObserve
			f.onDashboard = false
			f.buildPanels = 0
			f.continued = true
		}), []Action{ActionPin, ActionArchive}},
		{"failed investigate", base(func(f *runFacts) { f.source = SourceInvestigate; f.buildPanels = 0 }), []Action{ActionRetry, ActionPin, ActionArchive}},
		{"failed investigate with a build is never continued", base(func(f *runFacts) { f.source = SourceInvestigate }), []Action{ActionKeepBuild, ActionRetry, ActionPin, ActionArchive}},
		{"succeeded comment", base(func(f *runFacts) { f.state = StateSucceeded; f.source = SourceComment; f.buildPanels = 0 }), []Action{ActionPin, ActionArchive}},
		{"succeeded comment for a non-requester who may start", base(func(f *runFacts) {
			f.state = StateSucceeded
			f.source = SourceComment
			f.buildPanels = 0
			f.requester = false
		}), []Action{}},
		{"failed with the build saved", base(func(f *runFacts) { f.buildSaved = true }), []Action{ActionRetry, ActionPin, ActionArchive}},
		{"failed prompt without a dashboard", base(func(f *runFacts) { f.onDashboard = false; f.buildPanels = 0 }), []Action{ActionPin, ActionArchive}},
		{"succeeded observe retries", base(func(f *runFacts) { f.state = StateSucceeded; f.source = SourceObserve; f.buildPanels = 0 }), []Action{ActionRetry, ActionPin, ActionArchive}},
		{"failed suggest", base(func(f *runFacts) { f.source = SourceSuggest; f.buildPanels = 0 }), []Action{ActionPin, ActionArchive}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := allowedActions(c.facts)
			if got == nil || !slices.Equal(got, c.want) {
				t.Errorf("allowedActions = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestNoOneAsksToExpireARun(t *testing.T) {
	for _, state := range allStates {
		f := runFacts{state: state, source: SourceObserve, requester: true, mayStart: true, buildPanels: 1}
		if slices.Contains(allowedActions(f), ActionExpire) {
			t.Errorf("allowedActions of a %s run contains expire", state)
		}
	}
}

func TestActionStrings(t *testing.T) {
	if got := actionStrings(nil); got == nil || len(got) != 0 {
		t.Errorf("actionStrings(nil) = %#v, want an empty non-nil slice", got)
	}
	if got := actionStrings([]Action{ActionCancel, ActionPin}); !slices.Equal(got, []string{"cancel", "pin"}) {
		t.Errorf("actionStrings = %v", got)
	}
}
