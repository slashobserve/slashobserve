package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func callClaimTool(t *testing.T, claimer *claims.Claimer, name, input string) (Result, error) {
	t.Helper()
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Name: "n", Role: auth.RoleViewer})
	if claimer != nil {
		ctx = claims.WithClaimer(ctx, *claimer)
	}
	return New(Deps{}).Call(ctx, name, []byte(input))
}

func TestClaimToolsRejectABadInput(t *testing.T) {
	claimer := &claims.Claimer{Kind: revisions.AssignGrant, ID: "g1", Person: "u1"}
	longText := strings.Repeat("x", maxRunTextRunes+1)
	cases := map[string]struct{ tool, input string }{
		"wait above 30":          {nameClaimRun, `{"wait_seconds":31}`},
		"negative wait":          {nameClaimRun, `{"wait_seconds":-1}`},
		"fractional wait":        {nameClaimRun, `{"wait_seconds":1.5}`},
		"unknown claim field":    {nameClaimRun, `{"wait":5}`},
		"report without run":     {nameReportRun, `{"attempt":1}`},
		"report bad uuid":        {nameReportRun, `{"run_id":"nope","attempt":1}`},
		"report attempt zero":    {nameReportRun, `{"run_id":"` + runID + `","attempt":0}`},
		"report missing attempt": {nameReportRun, `{"run_id":"` + runID + `"}`},
		"report long note":       {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"note":"` + longText + `"}`},
		"report unknown field":   {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"extra":1}`},
		"finish bad uuid":        {nameFinishRun, `{"run_id":"nope","attempt":1,"outcome":"succeeded"}`},
		"finish attempt zero":    {nameFinishRun, `{"run_id":"` + runID + `","attempt":0,"outcome":"succeeded"}`},
		"finish unknown outcome": {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"done"}`},
		"finish no outcome":      {nameFinishRun, `{"run_id":"` + runID + `","attempt":1}`},
		"finish long message":    {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","message":"` + longText + `"}`},
		"finish unknown field":   {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","why":"x"}`},
		"finish unknown usage":   {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","usage":{"tokens":1}}`},
		"finish bad usage type":  {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","usage":{"model":3}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := callClaimTool(t, claimer, tc.tool, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q, want %q", code, codeInvalidRequest)
			}
		})
	}
}

func TestClaimToolsRefuseACallerThatIsNotAClaimer(t *testing.T) {
	for tool, input := range map[string]string{
		nameClaimRun:  `{}`,
		nameReportRun: `{"run_id":"` + runID + `","attempt":1}`,
		nameFinishRun: `{"run_id":"` + runID + `","attempt":1,"outcome":"failed"}`,
	} {
		t.Run(tool, func(t *testing.T) {
			res, err := callClaimTool(t, nil, tool, input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeNotAWorker {
				t.Fatalf("code %q, want %q", code, codeNotAWorker)
			}
		})
	}
}

func TestClaimToolsNeedTheClaimsService(t *testing.T) {
	claimer := &claims.Claimer{Kind: revisions.AssignGrant, ID: "g1", Person: "u1"}
	_, err := callClaimTool(t, claimer, nameClaimRun, `{"wait_seconds":0}`)
	if !errors.Is(err, errClaimsNotConfigured) {
		t.Fatalf("err = %v, want errClaimsNotConfigured", err)
	}
}

func TestClaimErrorsBecomeToolErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"not a worker":    {claims.ErrNotAWorker, codeNotAWorker},
		"not your run":    {claims.ErrNotYourRun, codeNotYourRun},
		"invalid outcome": {claims.ErrInvalidOutcome, codeInvalidRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := claimFailure(tc.err)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != tc.code {
				t.Fatalf("code %q, want %q", code, tc.code)
			}
		})
	}
	boom := errors.New("boom")
	if _, err := claimFailure(boom); !errors.Is(err, boom) {
		t.Fatalf("an unknown error must be returned, got %v", err)
	}
}

func TestClaimToolsAreMCPOnlyAndNeverOfferedInARun(t *testing.T) {
	byName := toolsByName(t)
	for _, name := range []string{nameClaimRun, nameReportRun, nameFinishRun} {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
			if tool.OfferedIn(mode) {
				t.Errorf("%s is offered in %s runs", name, mode)
			}
		}
	}
}
