package auth

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestGroupsClaim(t *testing.T) {
	many := make([]string, 250)
	for i := range many {
		many[i] = fmt.Sprintf("g%d", i)
	}
	manyJSON, err := json.Marshal(many)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"absent claim", ``, []string{}},
		{"null", `null`, []string{}},
		{"string instead of array", `"eng"`, []string{}},
		{"object instead of array", `{"eng":true}`, []string{}},
		{"empty array", `[]`, []string{}},
		{"plain names", `["eng","ops"]`, []string{"eng", "ops"}},
		{"non-string items are dropped", `["eng",1,null,true,["x"],{"a":1},"ops"]`, []string{"eng", "ops"}},
		{"empty strings are dropped", `["","eng"]`, []string{"eng"}},
		{"control characters drop the name", `["a\u0000b","a\nb","a\u007fb","eng"]`, []string{"eng"}},
		{"256 bytes is kept", `["` + strings.Repeat("a", 256) + `"]`, []string{strings.Repeat("a", 256)}},
		{"257 bytes is dropped", `["` + strings.Repeat("a", 257) + `","eng"]`, []string{"eng"}},
		{"limit counts bytes not characters", `["` + strings.Repeat("é", 129) + `","eng"]`, []string{"eng"}},
		{"first 200 are kept", string(manyJSON), many[:200]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupsClaim(json.RawMessage(tt.raw))
			if got == nil {
				t.Fatal("groupsClaim returned nil, want a non-nil slice")
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("groupsClaim = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupsScopeIsRequestedOnlyWhenTheProviderListsIt(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want bool
	}{
		{"listed", []string{"openid", "email", "groups"}, true},
		{"not listed", []string{"openid", "email", "profile"}, false},
		{"similar name", []string{"groups_extra", "Groups"}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsGroupsScope(tt.in); got != tt.want {
				t.Errorf("supportsGroupsScope(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
