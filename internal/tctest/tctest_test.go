package tctest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

type fakeContainer struct {
	testcontainers.Container
	terminated int
}

func (f *fakeContainer) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	f.terminated++
	return nil
}

var errReaperRace = errors.New(`create container: reaper: from container "f2ac365e": wait for reaper f2ac365e: unexpected container status "created"`)

func TestIsReaperRace(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"shared reaper not started yet", errReaperRace, true},
		{
			"reaper dies while starting",
			errors.New(`start postgres: run postgres: generic container: create container: reaper: new reaper: run container: started hook: wait until ready: mapped port: check target: retries: 10, port: "invalid port", last err: unexpected container status "removing": could not start container`),
			true,
		},
		{
			"reaper exits while starting",
			errors.New(`create container: reaper: new reaper: run container: started hook: wait until ready: mapped port: check target: retries: 3, port: "invalid port", last err: container exited with code 1: could not start container`),
			true,
		},
		{
			"reaper failure joined with its termination error",
			errors.Join(errors.New(`reaper: new reaper: run container: wait until ready: unexpected container status "removing"`), errors.New("terminate: boom")),
			true,
		},
		{
			"wrapped reaper failure",
			fmt.Errorf("start clickhouse: %w", errors.New(`reaper: new reaper: run container: unexpected container status "removing"`)),
			true,
		},
		{"reaper cannot be created", errors.New("create container: reaper: new reaper: run container: pull access denied"), false},
		{"reaper container out of memory", errors.New("reaper: new reaper: run container: wait until ready: container crashed with out-of-memory (OOMKilled)"), false},
		{"container under test exits", errors.New("start postgres: run postgres: generic container: started hook: wait until ready: container exited with code 1: could not start container"), false},
		{"container under test is removed", errors.New(`run postgres: wait until ready: mapped port: check target: last err: unexpected container status "removing"`), false},
		{"reaper connect fails", errors.New("reaper connect: dial tcp: connection refused"), false},
		{"context cancelled", context.Canceled, false},
		{"pull failure", errors.New("pull access denied"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isReaperRace(c.err); got != c.want {
				t.Errorf("isReaperRace(%q) = %t, want %t", c.err, got, c.want)
			}
		})
	}
}

func TestRetryReturnsOtherErrorsAfterOneCall(t *testing.T) {
	want := errors.New("pull access denied")
	calls := 0
	c := &fakeContainer{}
	got, err := retry(t.Context(), 3, time.Millisecond, func() (*fakeContainer, error) {
		calls++
		return c, want
	})
	if !errors.Is(err, want) || got != c || calls != 1 || c.terminated != 0 {
		t.Errorf("got %v, %v after %d calls, %d terminations", got, err, calls, c.terminated)
	}
}

func TestRetryStopsAtTheCapAndTerminatesEachFailedContainer(t *testing.T) {
	var made []*fakeContainer
	got, err := retry(t.Context(), 3, time.Millisecond, func() (*fakeContainer, error) {
		c := &fakeContainer{}
		made = append(made, c)
		return c, errReaperRace
	})
	if !errors.Is(err, errReaperRace) || len(made) != 3 || got != made[2] {
		t.Fatalf("got %v, %v after %d calls", got, err, len(made))
	}
	for i, c := range made[:2] {
		if c.terminated != 1 {
			t.Errorf("failed container %d terminated %d times, want 1", i, c.terminated)
		}
	}
	if made[2].terminated != 0 {
		t.Errorf("the returned container was terminated %d times, the caller owns it", made[2].terminated)
	}
}

func TestRetrySucceedsAfterAReaperRace(t *testing.T) {
	calls := 0
	_, err := retry(t.Context(), 3, time.Millisecond, func() (*fakeContainer, error) {
		calls++
		if calls == 1 {
			return &fakeContainer{}, errReaperRace
		}
		return &fakeContainer{}, nil
	})
	if err != nil || calls != 2 {
		t.Errorf("err %v after %d calls", err, calls)
	}
}

func TestRetryReturnsEarlyWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := retry(ctx, 3, time.Hour, func() (*fakeContainer, error) {
		calls++
		cancel()
		return &fakeContainer{}, errReaperRace
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Errorf("err %v after %d calls", err, calls)
	}

	calls = 0
	if _, err := retry(ctx, 3, time.Millisecond, func() (*fakeContainer, error) {
		calls++
		return nil, nil
	}); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Errorf("cancelled context: err %v after %d calls", err, calls)
	}
}
