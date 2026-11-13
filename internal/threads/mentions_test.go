package threads

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	adaID     = "11111111-1111-4111-8111-111111111111"
	boID      = "22222222-2222-4222-8222-222222222222"
	grantID   = "66666666-6666-4666-8666-666666666666"
	accountID = "77777777-7777-4777-8777-777777777777"
)

func TestParseMentions(t *testing.T) {
	user := func(name, id string) string { return "@[" + name + "](user:" + id + ")" }
	grant := func(name, id string) string { return "@[" + name + "](grant:" + id + ")" }
	account := func(name, id string) string { return "@[" + name + "](service_account:" + id + ")" }
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
		{"a connected agent", "ask " + grant("Codex", grantID), []string{"grant:" + grantID}},
		{"a service account", "ask " + account("ci-bot", accountID), []string{"service_account:" + accountID}},
		{"all three kinds and the agent", user("Ada", adaID) + grant("Codex", grantID) + account("ci-bot", accountID) + "@observe", []string{"user:" + adaID, "grant:" + grantID, "service_account:" + accountID, "agent:observe"}},
		{"upper case grant uuid is the same grant", grant("Codex", strings.ToUpper(grantID)) + grant("Codex", grantID), []string{"grant:" + grantID}},
		{"agent name inside a grant token is not the agent", grant("@observe", grantID), []string{"grant:" + grantID}},
		{"malformed grant and account tokens stay text", "@[Codex](grant:not-a-uuid) @[ci](service_account:" + grantID + "9)", nil},
		{"unknown token kind stays text", "@[Codex](agent:" + grantID + ")", nil},
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

func TestAgentsKeepsOnlyTheMentionsThatCanTakeARun(t *testing.T) {
	body := "@[Ada](user:" + adaID + ") @[Codex](grant:" + grantID + ") @[ci](service_account:" + accountID + ") @observe"
	got, err := ParseMentions(body)
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, m := range got.Agents() {
		refs = append(refs, m.Kind()+":"+m.RefID())
	}
	if want := "grant:" + grantID + ",service_account:" + accountID + ",agent:observe"; strings.Join(refs, ",") != want {
		t.Errorf("agents = %v, want %s", refs, want)
	}
	only, err := ParseMentions("@[Ada](user:" + adaID + ") and @observe")
	if err != nil || len(only.Agents()) != 1 || only.Agents()[0].Kind() != "agent" {
		t.Errorf("agents of a user and the agent = %+v, err %v", only.Agents(), err)
	}
	if none, err := ParseMentions("@[Ada](user:" + adaID + ")"); err != nil || len(none.Agents()) != 0 {
		t.Errorf("agents of a user alone = %+v, err %v", none.Agents(), err)
	}
}

func TestWithoutMentionRemovesOnlyTheNamedAgent(t *testing.T) {
	grant := Mention{GrantID: mustParse(t, grantID)}
	tests := []struct {
		name   string
		body   string
		target Mention
		want   string
	}{
		{"the handle at the start", "@observe why is checkout slow?", Mention{Agent: AgentObserve}, "why is checkout slow?"},
		{"the handle in the middle", "why is checkout slow, @observe, today?", Mention{Agent: AgentObserve}, "why is checkout slow, , today?"},
		{"every occurrence of the handle", "@observe check @observe", Mention{Agent: AgentObserve}, "check"},
		{"a longer handle stays", "@observers check @observe", Mention{Agent: AgentObserve}, "@observers check"},
		{"the handle inside a user token name stays", "@[@observe](user:" + adaID + ") @observe go", Mention{Agent: AgentObserve}, "@[@observe](user:" + adaID + ")  go"},
		{"a grant token", "@[Codex](grant:" + grantID + ") review this", grant, "review this"},
		{"a grant token with an upper case uuid", "review @[Codex](grant:" + strings.ToUpper(grantID) + ") this", grant, "review  this"},
		{"another grant stays", "@[Other](grant:" + accountID + ") @[Codex](grant:" + grantID + ") go", grant, "@[Other](grant:" + accountID + ")  go"},
		{"only the token", "@[Codex](grant:" + grantID + ")", grant, ""},
		{"a user mention stays", "@[Ada](user:" + adaID + ") @observe look", Mention{Agent: AgentObserve}, "@[Ada](user:" + adaID + ")  look"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withoutMention(tt.body, tt.target); got != tt.want {
				t.Errorf("withoutMention = %q, want %q", got, tt.want)
			}
		})
	}
}
