package export

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakeLagSource struct {
	answers []lagAnswer
	calls   int
}

type lagAnswer struct {
	lag time.Duration
	err error
}

func (s *fakeLagSource) Lag(context.Context) (time.Duration, error) {
	a := s.answers[min(s.calls, len(s.answers)-1)]
	s.calls++
	return a.lag, a.err
}

type lagGauge struct{ values []time.Duration }

func (g *lagGauge) ExportLag(d time.Duration) { g.values = append(g.values, d) }

func watchFor(t *testing.T, answers []lagAnswer, ticks int) (*lagGauge, string, []time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	gauge, logs := &lagGauge{}, &bytes.Buffer{}
	var waits []time.Duration
	w := NewLagWatcher(&fakeLagSource{answers: answers}, gauge, slog.New(slog.NewTextHandler(logs, nil)))
	w.Wait = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(waits) == ticks {
			cancel()
		}
		return ctx.Err()
	}
	w.Run(ctx)
	return gauge, logs.String(), waits
}

func TestTheLagWatcherReportsAtOnceAndOnEveryTick(t *testing.T) {
	gauge, _, waits := watchFor(t, []lagAnswer{{lag: time.Minute}, {lag: 2 * time.Minute}, {lag: 0}}, 3)
	want := []time.Duration{time.Minute, 2 * time.Minute, 0}
	if len(gauge.values) != 3 || gauge.values[0] != want[0] || gauge.values[1] != want[1] || gauge.values[2] != want[2] {
		t.Fatalf("reported %v, want %v", gauge.values, want)
	}
	for _, w := range waits {
		if w != LagEvery {
			t.Fatalf("waits = %v", waits)
		}
	}
}

func TestAFailedLagReadKeepsTheLastValueAndLogsOnlyAFixedMessage(t *testing.T) {
	gauge, logs, _ := watchFor(t, []lagAnswer{{lag: time.Minute}, {err: errors.New("password=hunter2")}}, 2)
	if len(gauge.values) != 1 || gauge.values[0] != time.Minute {
		t.Fatalf("reported %v", gauge.values)
	}
	if !strings.Contains(logs, "audit export lag unavailable") || !strings.Contains(logs, "audit_export_lag_failed") || strings.Contains(logs, "hunter2") {
		t.Fatalf("log = %s", logs)
	}
}
