package folders

import "testing"

func TestLevelsGeneralPicksTheGeneralFolderAbilities(t *testing.T) {
	general := folderLevel{Folder: Folder{Title: "General", General: true}, Abilities: AbilitiesFor(LevelComment)}
	other := folderLevel{Folder: Folder{Title: "Ops"}, Abilities: AbilitiesFor(LevelAdmin)}
	cases := []struct {
		name    string
		entries []folderLevel
		want    Abilities
	}{
		{"no folders", nil, Abilities{}},
		{"General is not visible", []folderLevel{other}, Abilities{}},
		{"General among others", []folderLevel{other, general}, AbilitiesFor(LevelComment)},
		{"only General", []folderLevel{general}, AbilitiesFor(LevelComment)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Levels{entries: tc.entries}).General(); got != tc.want {
				t.Fatalf("General() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
