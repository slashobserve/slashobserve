package threads

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	adaID = "11111111-1111-4111-8111-111111111111"
	boID  = "22222222-2222-4222-8222-222222222222"
)

func TestParseMentions(t *testing.T) {
	user := func(name, id string) string { return "@[" + name + "](user:" + id + ")" }
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"no mention", "plain text", nil},
		{"one user", "please look " + user("Ada", adaID), []string{"user:" + adaID}},
		{"the agent", "@observe why is it slow?", []string{"agent:observe"}},
		{"a user then the agent", user("Ada", adaID) + " and @observe", []string{"user:" + adaID, "agent:observe"}},
		{"duplicates collapse", user("Ada", adaID) + " " + user("Ada again", adaID) + " @observe @observe", []string{"user:" + adaID, "agent:observe"}},
		{"upper case uuid is the same user", user("Ada", strings.ToUpper(adaID)), []string{"user:" + adaID}},
		{"agent before punctuation", "ping @observe, then @observe.", []string{"agent:observe"}},
		{"agent in parentheses", "(@observe)", []string{"agent:observe"}},
		{"agent at the end", "thanks @observe", []string{"agent:observe"}},
		{"agent in backticks is still plain text", "run `@observe` now", []string{"agent:observe"}},
		{"longer handle is not the agent", "ask @observers", nil},
		{"handle with a suffix is not the agent", "ask @observe_bot or @observe-bot", nil},
		{"email address is not the agent", "mail email@observe.io", nil},
		{"agent name inside a user token is not the agent", user("@observe", adaID), []string{"user:" + adaID}},
		{"broken token stays text", "@[Ada](user:not-a-uuid) @[Ada(user:" + adaID + ")", nil},
		{"token without a name", "@[](user:" + adaID + ")", []string{"user:" + adaID}},
		{"two users in order", user("Bo", boID) + user("Ada", adaID), []string{"user:" + boID, "user:" + adaID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMentions(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			var refs []string
			for _, m := range got {
				refs = append(refs, m.Kind()+":"+m.RefID())
			}
			if strings.Join(refs, ",") != strings.Join(tt.want, ",") {
				t.Errorf("mentions = %v, want %v", refs, tt.want)
			}
		})
	}
}

func TestParseMentionsKeepsTheTokenNameForTheCaller(t *testing.T) {
	got, err := ParseMentions("@[Ada Lovelace](user:" + adaID + ")")
	if err != nil || len(got) != 1 || got[0].Name != "Ada Lovelace" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestParseMentionsAllowsTwentyAndRejectsTwentyOne(t *testing.T) {
	token := func(n int) string { return fmt.Sprintf("@[U%d](user:%08d-1111-4111-8111-111111111111) ", n, n) }
	body := ""
	for n := range MaxMentions - 1 {
		body += token(n)
	}
	if got, err := ParseMentions(body + "@observe"); err != nil || len(got) != MaxMentions {
		t.Fatalf("20 mentions: %d, err %v", len(got), err)
	}
	_, err := ParseMentions(body + token(MaxMentions) + "@observe")
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) || invalid.Code != CodeTooManyMentions {
		t.Fatalf("21 mentions: err = %v, want too_many_mentions", err)
	}
	if got, err := ParseMentions(body + token(0) + token(1) + "@observe @observe"); err != nil || len(got) != MaxMentions {
		t.Fatalf("duplicates must not count twice: %d, err %v", len(got), err)
	}
}
