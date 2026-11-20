package savedsearches

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type typedOnly struct {
	datasource.Datasource
	typ string
}

func (d typedOnly) Type() string { return d.typ }

func TestASavedSearchOnAnAdminsOnlyDatasourceIsRefusedLikeAMissingOne(t *testing.T) {
	reg, err := datasource.NewRegistry(nil, datasource.Instance{UID: "prom", Datasource: typedOnly{typ: "prometheus"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: typedOnly{typ: "prometheus"}}); err != nil {
		t.Fatal(err)
	}
	s := &Service{sources: reg}
	search := func(uid string) validated {
		return validated{CreateInput: CreateInput{Kind: KindMetrics, Title: "Audit", DatasourceUID: uid, Query: ptr("up")}}
	}
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			ctx := datasource.WithAccess(reading(t.Context()), datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "u", Role: role}})
			got := s.checkDatasource(ctx, search("audit"))
			if role == auth.RoleAdmin {
				if got != nil {
					t.Fatalf("an Admin's saved search was refused: %v", got)
				}
				return
			}
			missing := s.checkDatasource(ctx, search("nope"))
			if got == nil || missing == nil {
				t.Fatalf("got %v, missing %v", got, missing)
			}
			var restricted, absent *InvalidInputError
			if !errors.As(got, &restricted) || !errors.As(missing, &absent) || restricted.Code != absent.Code {
				t.Fatalf("restricted %v, missing %v", got, missing)
			}
		})
	}
}
