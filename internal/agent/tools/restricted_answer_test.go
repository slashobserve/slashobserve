package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestARestrictedAnswerRefusalReachesAnAgentAsRestrictedAnswerWithWhy(t *testing.T) {
	err := fmt.Errorf("share: %w", folders.ErrRestrictedAnswer)
	for name, refuse := range map[string]func(context.Context, error) (Result, bool){"a save": restrictedRefusal, "a share": shareRefusal} {
		res, ok := refuse(t.Context(), err)
		if !ok || errorCode(t, res) != folders.CodeRestrictedAnswer {
			t.Fatalf("%s: refusal = %+v, handled %v", name, res, ok)
		}
		if body := res.Content.(errorBody); !strings.Contains(body.Message, "only Admins") {
			t.Fatalf("%s: message %q does not say why", name, body.Message)
		}
	}
}

func TestAnApplyOrAStartRefusedForARestrictedAnswerReachesAnAgentAsRestrictedAnswer(t *testing.T) {
	err := fmt.Errorf("enqueue: %w", folders.ErrRestrictedAnswer)
	res, ok := applyRefusal(t.Context(), err)
	if !ok || errorCode(t, res) != folders.CodeRestrictedAnswer {
		t.Fatalf("apply refusal = %+v, handled %v", res, ok)
	}
	res, callErr := runFailure(t.Context(), err)
	if callErr != nil || errorCode(t, res) != folders.CodeRestrictedAnswer {
		t.Fatalf("start refusal = %+v, %v", res, callErr)
	}
}

func TestRestrictedContentAdviceMatchesTheCaller(t *testing.T) {
	err := folders.RestrictedSave(folders.ContentDashboard, "audit")
	for name, tc := range map[string]struct {
		ctx  context.Context
		want string
		not  string
	}{
		"an Admin":  {auth.WithPrincipal(t.Context(), auth.Principal{Role: auth.RoleAdmin}), "or you can set a folder to Admins only", "ask an Admin"},
		"an Editor": {auth.WithPrincipal(t.Context(), auth.Principal{Role: auth.RoleEditor}), "or ask an Admin to set a folder to Admins only", "you can set"},
		"no caller": {t.Context(), "or ask an Admin to set a folder to Admins only", "you can set"},
	} {
		res, ok := restrictedRefusal(tc.ctx, err)
		if !ok {
			t.Fatalf("%s: not handled", name)
		}
		message := res.Content.(errorBody).Message
		if !strings.Contains(message, tc.want) || strings.Contains(message, tc.not) {
			t.Fatalf("%s: message %q", name, message)
		}
	}
}
