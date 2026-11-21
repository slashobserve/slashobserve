package revisions

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/usertext"
)

func ptr[T any](v T) *T { return &v }

func TestStartRefusesABadRequestBeforeTouchingAnything(t *testing.T) {
	cases := map[string]struct {
		req  StartRequest
		want string
	}{
		"no prompt":                   {StartRequest{}, "prompt is required"},
		"blank prompt":                {StartRequest{Prompt: ptr("   ")}, "prompt must not be empty"},
		"unknown kind":                {StartRequest{Prompt: ptr("x"), Kind: ptr("rca")}, "kind must be build, ask or investigate"},
		"investigation fields":        {StartRequest{Prompt: ptr("x"), PanelID: ptr("p")}, "need kind ask or investigate"},
		"observation on a build":      {StartRequest{Prompt: ptr("x"), Kind: ptr("build"), ObservationID: ptr("o")}, "goes with kind ask or investigate"},
		"base version needs a board":  {StartRequest{Prompt: ptr("x"), BaseVersion: ptr(2)}, "base_version needs dashboard_id"},
		"bad dashboard id":            {StartRequest{Prompt: ptr("x"), DashboardID: ptr("nope")}, "dashboard_id must be a UUID"},
		"base version range":          {StartRequest{Prompt: ptr("x"), DashboardID: ptr("11111111-2222-3333-4444-555555555555"), BaseVersion: ptr(0)}, "positive 32-bit"},
		"follows a chat and a board":  {StartRequest{Prompt: ptr("x"), DashboardID: ptr("11111111-2222-3333-4444-555555555555"), FollowsID: ptr("11111111-2222-3333-4444-555555555555")}, "follows_id and dashboard_id exclude each other"},
		"follows a bad id":            {StartRequest{Prompt: ptr("x"), FollowsID: ptr("nope")}, "follows_id must be a UUID"},
		"investigate with a version":  {StartRequest{Prompt: ptr("x"), Kind: ptr("investigate"), BaseVersion: ptr(1)}, "not allowed for kind investigate"},
		"panel without dashboard":     {StartRequest{Prompt: ptr("x"), Kind: ptr("investigate"), PanelID: ptr("p")}, "panel_id needs dashboard_id"},
		"half a window":               {StartRequest{Prompt: ptr("x"), Kind: ptr("investigate"), From: ptr("now-1h")}, "go together"},
		"an ask about nothing":        {StartRequest{Prompt: ptr("x"), Kind: ptr("ask")}, "kind ask needs dashboard_id or observation_id"},
		"an ask about two things":     {StartRequest{Prompt: ptr("x"), Kind: ptr("ask"), DashboardID: ptr("11111111-2222-3333-4444-555555555555"), ObservationID: ptr("11111111-2222-3333-4444-555555555555")}, "not both"},
		"an ask with a version":       {StartRequest{Prompt: ptr("x"), Kind: ptr("ask"), BaseVersion: ptr(1)}, "not allowed for kind ask"},
		"a bad observation of an ask": {StartRequest{Prompt: ptr("x"), Kind: ptr("ask"), ObservationID: ptr("o")}, "observation_id must be a UUID"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := (&Service{}).Start(reading(t.Context()), auth.Principal{}, tc.req)
			var invalid *StartError
			if !errors.As(err, &invalid) || !strings.Contains(invalid.Message, tc.want) {
				t.Fatalf("err = %v, want a StartError containing %q", err, tc.want)
			}
		})
	}
}

func TestStartKeepsTheReasonATextRuleGave(t *testing.T) {
	_, err := (&Service{}).Start(reading(t.Context()), auth.Principal{}, StartRequest{Prompt: ptr("\u200b \u200b")})
	reason, ok := usertext.ReasonOf(err)
	if !ok || reason != usertext.ReasonInvisibleOnly {
		t.Fatalf("reason = %q, %v for %v", reason, ok, err)
	}
}
