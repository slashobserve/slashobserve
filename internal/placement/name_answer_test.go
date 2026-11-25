package placement

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestOnlyAnAdminOrTheRequesterMayHaveTheAnswerNamed(t *testing.T) {
	requester := uuid.New()
	run := answerRun{RequesterOf: string(auth.KindUser), RequesterID: pgtype.UUID{Bytes: requester, Valid: true}}
	cases := map[string]struct {
		caller auth.Principal
		known  bool
		want   bool
	}{
		"an Admin":                       {auth.Principal{Kind: auth.KindUser, ID: uuid.NewString(), Role: auth.RoleAdmin}, true, true},
		"the user who asked":             {auth.Principal{Kind: auth.KindUser, ID: requester.String(), Role: auth.RoleEditor}, true, true},
		"another Editor":                 {auth.Principal{Kind: auth.KindUser, ID: uuid.NewString(), Role: auth.RoleEditor}, true, false},
		"a service account with that id": {auth.Principal{Kind: auth.KindServiceAccount, ID: requester.String(), Role: auth.RoleEditor}, true, false},
		"no caller in the context":       {auth.Principal{}, false, false},
		"an Admin role without a caller": {auth.Principal{Role: auth.RoleAdmin}, false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mayNameAnswer(tc.caller, tc.known, run); got != tc.want {
				t.Fatalf("mayNameAnswer = %v, want %v", got, tc.want)
			}
		})
	}
}
