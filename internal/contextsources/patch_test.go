package contextsources

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func sourceWithSecret(t *testing.T, s *Service) dbsqlc.ContextSource {
	t.Helper()
	id := newID()
	sealedSecret, err := s.seal(id, "token-value")
	if err != nil {
		t.Fatal(err)
	}
	return dbsqlc.ContextSource{
		ID: id, Uid: "docs", Name: "Docs", Url: "https://mcp.example.com/mcp", Audience: "everyone",
		SecretHeader: pgtype.Text{String: "Authorization", Valid: true}, Secret: sealedSecret,
	}
}

func str(s string) *string { return &s }

func TestPatchRefusesWhatWouldBreakTheSecretPair(t *testing.T) {
	s := &Service{key: testKey}
	row := sourceWithSecret(t, s)
	cases := map[string]Patch{
		"empty header with a stored secret": {SecretHeader: str("")},
		"blank header with a stored secret": {SecretHeader: str("  ")},
		"empty secret":                      {Secret: str("")},
		"empty secret with a new header":    {Secret: str(""), SecretHeader: str("X-Key")},
	}
	for name, p := range cases {
		_, err := s.patched(row, p)
		var invalidErr *InvalidError
		if !errors.As(err, &invalidErr) {
			t.Errorf("%s: err = %v, want InvalidError", name, err)
		}
	}
}

func TestPatchAllowsClearingTheSecretAndItsHeader(t *testing.T) {
	s := &Service{key: testKey}
	row := sourceWithSecret(t, s)
	next, err := s.patched(row, Patch{SecretHeader: str(""), ClearSecret: true})
	if err != nil {
		t.Fatal(err)
	}
	if next.params.Secret != nil || next.params.SecretHeader.Valid {
		t.Fatalf("params = %+v, want no secret and no header", next.params)
	}
	if !next.credentials {
		t.Fatal("clearing a stored secret should count as a credentials change")
	}
}

func TestPatchWithoutSecretHeaderOnASourceWithoutSecretIsAccepted(t *testing.T) {
	s := &Service{key: testKey}
	row := dbsqlc.ContextSource{ID: newID(), Uid: "docs", Name: "Docs", Url: "https://mcp.example.com/mcp", Audience: "everyone"}
	if _, err := s.patched(row, Patch{Name: str("Renamed")}); err != nil {
		t.Fatal(err)
	}
}

func TestPatchSetsKeepsAndClearsTheDescription(t *testing.T) {
	s := &Service{key: testKey}
	row := dbsqlc.ContextSource{ID: newID(), Uid: "docs", Name: "Docs", Url: "https://mcp.example.com/mcp", Audience: "everyone", Description: pgtype.Text{String: "Runbooks", Valid: true}}

	kept, err := s.patched(row, Patch{Name: str("Renamed")})
	if err != nil || kept.params.Description.String != "Runbooks" || !kept.params.Description.Valid {
		t.Fatalf("kept = %+v, %v", kept.params.Description, err)
	}
	cleared, err := s.patched(row, Patch{Description: str("  ")})
	if err != nil || cleared.params.Description.Valid {
		t.Fatalf("cleared = %+v, %v", cleared.params.Description, err)
	}
	set, err := s.patched(row, Patch{Description: str("Wiki")})
	if err != nil || set.params.Description.String != "Wiki" {
		t.Fatalf("set = %+v, %v", set.params.Description, err)
	}
	if _, err := s.patched(row, Patch{Description: str("a\nb")}); err == nil {
		t.Fatal("a description with a line break must be refused")
	}
}
