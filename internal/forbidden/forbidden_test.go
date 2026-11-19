package forbidden

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestNeedKeepsTheSentinelAndCarriesItsBody(t *testing.T) {
	sentinel := errors.New("refused")
	err := fmt.Errorf("pin: %w", Need(sentinel, NeedsAccess("edit", "QA")))
	if !errors.Is(err, sentinel) {
		t.Fatal("the wrapped error no longer matches its sentinel")
	}
	if got := BodyOf(err); got != NeedsAccess("edit", "QA") {
		t.Fatalf("body = %+v", got)
	}
	if got := BodyOf(sentinel); got != (Body{Error: RoleMessage, Code: Code}) {
		t.Fatalf("an error with no need answered %+v", got)
	}
}

func TestMessageSaysWhatWouldLetTheCallThrough(t *testing.T) {
	tests := []struct {
		body Body
		want string
	}{
		{NeedsAccess("edit", "Payments"), "your role does not allow this; it needs Edit on Payments"},
		{NeedsAccess("manage", ""), "your role does not allow this; it needs Manage on the folder"},
		{NeedsRole("editor"), "your role does not allow this; it needs the Editor role"},
		{BodyOf(errors.New("x")), "your role does not allow this"},
	}
	for _, tt := range tests {
		if got := tt.body.Message(); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.body, got, tt.want)
		}
	}
}
