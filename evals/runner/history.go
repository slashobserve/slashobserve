package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var historyReference = time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)

type dispatch struct {
	index        int
	awaitHistory bool
}

func historyNeed(sc Scenario) time.Duration {
	need := time.Duration(0)
	for _, from := range sc.Expect.TimeRangeFrom {
		if t, err := spec.ResolveTime(from, historyReference, false, spec.TimeOptions{}); err == nil {
			need = max(need, historyReference.Sub(t))
		}
	}
	return need
}

func dispatchPlan(scenarios []Scenario, awaitHistory bool) []dispatch {
	plan := make([]dispatch, len(scenarios))
	for i := range scenarios {
		plan[i].index = i
	}
	if !awaitHistory {
		return plan
	}
	need := func(d dispatch) time.Duration { return historyNeed(scenarios[d.index]) }
	slices.SortStableFunc(plan, func(a, b dispatch) int { return cmp.Compare(need(a), need(b)) })
	if first := slices.IndexFunc(plan, func(d dispatch) bool { return need(d) > 0 }); first >= 0 {
		plan[first].awaitHistory = true
	}
	return plan
}

func historyWaitArgs(profile string) []string {
	if profile == profileK8s {
		return []string{"wait-history", "--k8s"}
	}
	return []string{"wait-history"}
}

func scriptHistoryWaiter(script, profile string, stderr io.Writer) func(context.Context) error {
	return func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, script, historyWaitArgs(profile)...) //nolint:gosec // the script is the operator's --stack-script and the arguments come from a fixed list
		cmd.Stdout, cmd.Stderr = stderr, stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s wait-history: %w", script, err)
		}
		return nil
	}
}
