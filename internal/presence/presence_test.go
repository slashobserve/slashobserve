package presence_test

import (
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/presence"
)

func TestOf(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		seenAt  time.Time
		seen    bool
		working bool
		want    presence.State
	}{
		{"never seen and idle", time.Time{}, false, false, presence.Offline},
		{"asked a moment ago", now.Add(-5 * time.Second), true, false, presence.Listening},
		{"asked exactly 40 seconds ago", now.Add(-40 * time.Second), true, false, presence.Listening},
		{"asked 40 seconds and a nanosecond ago", now.Add(-40*time.Second - time.Nanosecond), true, false, presence.Offline},
		{"holds a run and asked long ago", now.Add(-time.Hour), true, true, presence.Working},
		{"holds a run and was never seen", time.Time{}, false, true, presence.Working},
		{"a seen time without the seen flag", now, false, false, presence.Offline},
	}
	for _, c := range cases {
		if got := presence.Of(c.seenAt, c.seen, c.working, now); got != c.want {
			t.Errorf("%s: Of = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestStatesListTheThreePresences(t *testing.T) {
	if got := presence.States.Names(); !slices.Equal(got, []string{"working", "listening", "offline"}) {
		t.Errorf("States.Names() = %v", got)
	}
}
