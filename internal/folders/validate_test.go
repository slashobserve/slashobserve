package folders

import (
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"plain title", "Checkout", "Checkout", false},
		{"surrounding spaces are trimmed", "  Checkout \t", "Checkout", false},
		{"inner spaces stay", "Checkout team", "Checkout team", false},
		{"empty", "", "", true},
		{"only spaces", "   ", "", true},
		{"100 characters", strings.Repeat("a", 100), strings.Repeat("a", 100), false},
		{"101 characters", strings.Repeat("a", 101), "", true},
		{"100 multi-byte characters", strings.Repeat("é", 100), strings.Repeat("é", 100), false},
		{"101 multi-byte characters", strings.Repeat("é", 101), "", true},
		{"newline inside", "a\nb", "", true},
		{"NUL inside", "a\x00b", "", true},
		{"tab inside", "a\tb", "", true},
		{"invalid UTF-8", "a\xffb", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanTitle(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

func str(s string) *string { return &s }

func TestParseEntries(t *testing.T) {
	const userA = "0192b3a4-0000-7000-8000-000000000001"
	const userB = "0192b3a4-0000-7000-8000-000000000002"
	many := make([]entryBody, 51)
	for i := range many {
		many[i] = entryBody{Group: str("g" + strings.Repeat("x", i)), Level: "view"}
	}
	cases := []struct {
		name    string
		in      []entryBody
		wantLen int
		wantErr bool
	}{
		{"no entries", nil, 0, false},
		{"one of each kind", []entryBody{
			{UserID: str(userA), Level: "admin"},
			{Email: str("Priya@Example.com"), Level: "view"},
			{Group: str("sre"), Level: "edit"},
		}, 3, false},
		{"50 entries", many[:50], 50, false},
		{"51 entries", many, 0, true},
		{"unknown level", []entryBody{{Group: str("sre"), Level: "owner"}}, 0, true},
		{"empty level", []entryBody{{Group: str("sre")}}, 0, true},
		{"none is not a level", []entryBody{{Group: str("sre"), Level: "none"}}, 0, true},
		{"no source", []entryBody{{Level: "view"}}, 0, true},
		{"user_id and group", []entryBody{{UserID: str(userA), Group: str("sre"), Level: "view"}}, 0, true},
		{"email and group", []entryBody{{Email: str("a@b.c"), Group: str("sre"), Level: "view"}}, 0, true},
		{"user_id that is not a UUID", []entryBody{{UserID: str("priya"), Level: "view"}}, 0, true},
		{"the same user twice", []entryBody{{UserID: str(userA), Level: "view"}, {UserID: str(userA), Level: "edit"}}, 0, true},
		{"two different users", []entryBody{{UserID: str(userA), Level: "view"}, {UserID: str(userB), Level: "edit"}}, 2, false},
		{"the same email in different case", []entryBody{{Email: str("a@b.c"), Level: "view"}, {Email: str(" A@B.C "), Level: "edit"}}, 0, true},
		{"empty email", []entryBody{{Email: str("  "), Level: "view"}}, 0, true},
		{"the same group twice", []entryBody{{Group: str("sre"), Level: "view"}, {Group: str("sre"), Level: "edit"}}, 0, true},
		{"groups differing in case are different groups", []entryBody{{Group: str("sre"), Level: "view"}, {Group: str("SRE"), Level: "edit"}}, 2, false},
		{"empty group", []entryBody{{Group: str(""), Level: "view"}}, 0, true},
		{"256 byte group", []entryBody{{Group: str(strings.Repeat("g", 256)), Level: "view"}}, 1, false},
		{"257 byte group", []entryBody{{Group: str(strings.Repeat("g", 257)), Level: "view"}}, 0, true},
		{"group with a control character", []entryBody{{Group: str("sre\n"), Level: "view"}}, 0, true},
		{"group with spaces inside", []entryBody{{Group: str("platform team"), Level: "view"}}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEntries(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != tc.wantLen {
				t.Errorf("entries = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestParseEntriesNormalizesEmailAndKeepsLevel(t *testing.T) {
	got, err := parseEntries([]entryBody{{Email: str("  Priya@Example.COM "), Level: "edit"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].email != "priya@example.com" || got[0].level != LevelEdit {
		t.Errorf("entry = %+v", got[0])
	}
}
