package prompts

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"collapses and trims spaces", "  Checkout   health ", "Checkout health", true},
		{"dash and underscore inside", "deploy-check_2", "deploy-check_2", true},
		{"unicode letters", "Ünïcode 名前", "Ünïcode 名前", true},
		{"a tab reads as a space", "tab\there", "tab here", true},
		{"64 runes", strings.Repeat("a", 64), strings.Repeat("a", 64), true},
		{"65 runes", strings.Repeat("a", 65), "", false},
		{"empty", "", "", false},
		{"only spaces", "   ", "", false},
		{"leading dash", "-lead", "", false},
		{"trailing underscore", "trail_", "", false},
		{"punctuation", "semi;colon", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeName(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeName(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCleanBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"trims", "  show checkout errors \n", "show checkout errors", true},
		{"keeps inner newlines", "line one\nline two", "line one\nline two", true},
		{"4000 runes", strings.Repeat("é", 4000), strings.Repeat("é", 4000), true},
		{"4001 runes", strings.Repeat("é", 4001), "", false},
		{"empty", "  ", "", false},
		{"nul", "a\x00b", "", false},
		{"bidi override", "a\u202eb", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := CleanBody(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("CleanBody(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCanEdit(t *testing.T) {
	const owner = "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11"
	cases := []struct {
		name   string
		caller auth.Principal
		want   bool
	}{
		{"the owner", auth.Principal{Kind: auth.KindUser, ID: owner}, true},
		{"another person", auth.Principal{Kind: auth.KindUser, ID: "0a1b2c3d-0000-4000-8000-000000000001"}, false},
		{"a service account with the owner's id", auth.Principal{Kind: auth.KindServiceAccount, ID: owner}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanEdit(tc.caller, owner); got != tc.want {
				t.Fatalf("CanEdit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPatch(t *testing.T) {
	name, body, shared := "New", "new body", true
	if !(Patch{}).Empty() {
		t.Fatal("a patch with nothing sent must be empty")
	}
	for _, p := range []Patch{{Name: &name}, {Body: &body}, {Shared: &shared}} {
		if p.Empty() {
			t.Fatalf("patch %+v must not be empty", p)
		}
	}
	gotName, gotBody, gotShared := Patch{Body: &body}.Apply("Old", "old body", false)
	if gotName != "Old" || gotBody != "new body" || gotShared {
		t.Fatalf("Apply = %q, %q, %v; want unsent fields kept", gotName, gotBody, gotShared)
	}
	gotName, gotBody, gotShared = Patch{Name: &name, Body: &body, Shared: &shared}.Apply("Old", "old body", false)
	if gotName != "New" || gotBody != "new body" || !gotShared {
		t.Fatalf("Apply = %q, %q, %v; want every sent field applied", gotName, gotBody, gotShared)
	}
}
