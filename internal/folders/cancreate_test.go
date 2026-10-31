package folders

import "testing"

func TestLevelsCanCreate(t *testing.T) {
	view := folderLevel{Abilities: Abilities{View: true}}
	comment := folderLevel{Abilities: Abilities{View: true, Comment: true}}
	create := folderLevel{Abilities: Abilities{View: true, Comment: true, Edit: true, Create: true}}
	cases := []struct {
		name    string
		entries []folderLevel
		want    bool
	}{
		{"no folders", nil, false},
		{"only View and Comment", []folderLevel{view, comment}, false},
		{"one Create among others", []folderLevel{view, create, comment}, true},
		{"only Create", []folderLevel{create}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Levels{entries: tc.entries}).CanCreate(); got != tc.want {
				t.Fatalf("CanCreate() = %v, want %v", got, tc.want)
			}
		})
	}
}
