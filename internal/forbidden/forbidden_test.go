package forbidden

import (
	"encoding/json"
	"testing"
)

func TestBodyCarriesOnlyTheFieldsItNames(t *testing.T) {
	tests := []struct {
		name string
		body Body
		want string
	}{
		{"role", NeedsRole("editor"), `{"error":"your role does not allow this","code":"forbidden_role","needed_role":"editor"}`},
		{"folder level", NeedsAccess("edit", "QA"), `{"error":"your role does not allow this","code":"forbidden_role","needed_access":"edit","folder_title":"QA"}`},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.body)
		if err != nil || string(got) != tt.want {
			t.Errorf("%s: %s, %v, want %s", tt.name, got, err, tt.want)
		}
	}
}
