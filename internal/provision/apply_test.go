package provision

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCheckPathAcceptsARootFileOrOneFolderDirectory(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"overview.json", true},
		{"Shop/checkout.json", true},
		{"Team %2F Payments/api.json", true},
		{"Größe/übersicht.json", true},
		{"", false},
		{".json", false},
		{"Shop/.json", false},
		{"overview.yaml", false},
		{"overview", false},
		{"/overview.json", false},
		{"Shop//checkout.json", false},
		{"Shop/", false},
		{"a/b/c.json", false},
		{"../checkout.json", false},
		{"./checkout.json", false},
		{"Shop/../checkout.json", false},
		{".git/config.json", false},
		{"Shop\\checkout.json", false},
		{"Shop/check\x00out.json", false},
		{"bad\xffutf8.json", false},
		{strings.Repeat("a", 508) + ".json", false},
	}
	for _, c := range cases {
		err := CheckPath(c.path)
		if (err == nil) != c.ok {
			t.Errorf("CheckPath(%q) = %v, want ok %v", c.path, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrInvalidPath) {
			t.Errorf("CheckPath(%q) = %v, want ErrInvalidPath", c.path, err)
		}
	}
}

func TestAdoptTakesOnlyASingleMatch(t *testing.T) {
	a := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	b := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	cases := []struct {
		name       string
		candidates []pgtype.UUID
		id         pgtype.UUID
		choice     adoption
	}{
		{"no match creates", nil, pgtype.UUID{}, adoptNone},
		{"one match is adopted", []pgtype.UUID{a}, a, adoptOne},
		{"two matches are ambiguous", []pgtype.UUID{a, b}, pgtype.UUID{}, adoptAmbiguous},
	}
	for _, c := range cases {
		id, choice := adopt(c.candidates)
		if id != c.id || choice != c.choice {
			t.Errorf("%s: adopt = %v, %v; want %v, %v", c.name, id, choice, c.id, c.choice)
		}
	}
}

func TestFolderTitleOfARootFileIsGeneral(t *testing.T) {
	cases := map[string]string{
		"overview.json":              "General",
		"Shop/checkout.json":         "Shop",
		"Team %2F Payments/api.json": "Team / Payments",
	}
	for path, want := range cases {
		if got := folderTitleOf(path); got != want {
			t.Errorf("folderTitleOf(%q) = %q, want %q", path, got, want)
		}
	}
}
