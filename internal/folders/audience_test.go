package folders

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

func TestAudienceOfPrincipals(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	tests := []struct {
		name      string
		principal auth.Principal
		wantAdmin bool
		wantKind  string
	}{
		{"viewer", auth.Principal{Kind: auth.KindUser, ID: id, Role: auth.RoleViewer}, false, "user"},
		{"admin", auth.Principal{Kind: auth.KindUser, ID: id, Role: auth.RoleAdmin}, true, "user"},
		{"admin key", auth.Principal{Kind: auth.KindServiceAccount, ID: id, Role: auth.RoleAdmin}, true, "service_account"},
		{"editor key", auth.Principal{Kind: auth.KindServiceAccount, ID: id, Role: auth.RoleEditor}, false, "service_account"},
	}
	want, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AudienceOf(tt.principal, Levels{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Admin != tt.wantAdmin || got.ReaderKind != tt.wantKind || got.ReaderID != want {
				t.Fatalf("got %+v", got)
			}
			if got.Folders == nil || len(got.Folders) != 0 {
				t.Fatalf("folders = %v, want an empty list", got.Folders)
			}
		})
	}
}

func TestAudienceOfRefusesAnUnparsablePrincipalID(t *testing.T) {
	for _, id := range []string{"", "not-a-uuid", "11111111111141118111111111111111"} {
		_, err := AudienceOf(auth.Principal{Kind: auth.KindUser, ID: id, Role: auth.RoleAdmin}, Levels{})
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("id %q: got %v, want ErrForbidden", id, err)
		}
	}
}
