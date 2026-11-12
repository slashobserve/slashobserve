package revisions

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func liveTextOf(t *testing.T, opts ObserveOptions, row dbsqlc.AgentRequest) (*string, string) {
	t.Helper()
	row.Steps = []byte("[]")
	reader := NewRunReader(&Service{logger: slog.New(slog.DiscardHandler)}, opts)
	out := reader.view(context.Background(), row, auth.Principal{}, buildState{}, nil, nil)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return out.LiveText, string(fields["live_text"])
}

func TestLiveTextShowsOnlyForARunningRunsCurrentAttempt(t *testing.T) {
	stored := map[string]struct {
		attempt int
		text    string
	}{runRequestID.String(): {attempt: 2, text: "Checking the p99 latency"}}
	opts := ObserveOptions{LiveText: func(runID string, attempt int) (string, bool) {
		kept, ok := stored[runID]
		return kept.text, ok && kept.attempt == attempt
	}}
	running := func(attempts int32) dbsqlc.AgentRequest {
		row := runRow(StatusRunning, SourcePrompt)
		row.Attempts = attempts
		return row
	}
	tests := []struct {
		name     string
		opts     ObserveOptions
		row      dbsqlc.AgentRequest
		wantText string
		wantNull bool
	}{
		{"running with text", opts, running(2), "Checking the p99 latency", false},
		{"succeeded with text in the store", opts, func() dbsqlc.AgentRequest {
			row := runRow(StatusSucceeded, SourcePrompt)
			row.Attempts = 2
			return row
		}(), "", true},
		{"running with text for another attempt", opts, running(3), "", true},
		{"running without a store", ObserveOptions{}, running(2), "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, encoded := liveTextOf(t, tc.opts, tc.row)
			if tc.wantNull {
				if got != nil || encoded != "null" {
					t.Fatalf("live_text = %v (%s), want null", got, encoded)
				}
				return
			}
			if got == nil || *got != tc.wantText {
				t.Fatalf("live_text = %v, want %q", got, tc.wantText)
			}
		})
	}
}
