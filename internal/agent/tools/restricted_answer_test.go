package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func TestARestrictedAnswerRefusalReachesAnAgentAsRestrictedAnswerWithWhy(t *testing.T) {
	err := fmt.Errorf("share: %w", folders.ErrRestrictedAnswer)
	for name, refuse := range map[string]func(error) (Result, bool){"a save": restrictedRefusal, "a share": shareRefusal} {
		res, ok := refuse(err)
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
	res, ok := applyRefusal(err)
	if !ok || errorCode(t, res) != folders.CodeRestrictedAnswer {
		t.Fatalf("apply refusal = %+v, handled %v", res, ok)
	}
	res, callErr := runFailure(err)
	if callErr != nil || errorCode(t, res) != folders.CodeRestrictedAnswer {
		t.Fatalf("start refusal = %+v, %v", res, callErr)
	}
}
