package activity

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestAppendRefusesAnUnknownTypeBeforeTouchingTheDatabase(t *testing.T) {
	user := actor.Actor{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve"}
	for _, typ := range []string{"", "comment.created", "dashboard.exploded"} {
		if _, err := Append(t.Context(), nil, Entry{Type: typ, Actor: user}, nil); err == nil {
			t.Errorf("type %q: expected an error", typ)
		}
	}
}

func TestAppendRefusesAnActorWhoseColumnsFail(t *testing.T) {
	bad := []actor.Actor{
		{Kind: auth.KindUser, ID: "not-a-uuid", Name: "Eve"},
		{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", RunID: "not-a-uuid"},
	}
	for _, a := range bad {
		if _, err := Append(t.Context(), nil, Entry{Type: TypeFindingCreated, Actor: a}, nil); err == nil {
			t.Errorf("actor %+v: expected an error", a)
		}
	}
}

func TestMarshalDataTurnsNilIntoAnEmptyObject(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, `{}`},
		{"nil map", map[string]any(nil), `{}`},
		{"map", map[string]any{"version": 3}, `{"version":3}`},
		{"struct", struct {
			Title string `json:"title"`
		}{"Checkout"}, `{"title":"Checkout"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalData(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
