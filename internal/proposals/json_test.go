package proposals

import (
	"encoding/json"
	"testing"
)

func TestSummaryJSONActorLabelsTheAuthor(t *testing.T) {
	const runID = "99999999-9999-4999-8999-999999999999"
	cases := []struct {
		name string
		item Item
		want string
	}{
		{
			name: "person on the web",
			item: Item{Author: Author{Kind: "user", ID: editorID, Name: "Ada"}, Origin: "web"},
			want: `{"kind":"user","id":"` + editorID + `","name":"Ada","origin":"web","client":null,"run_id":null,"label":"Ada"}`,
		},
		{
			name: "agent run for a person",
			item: Item{Author: Author{Kind: "user", ID: editorID, Name: "Ada"}, Origin: "web", RunID: runID},
			want: `{"kind":"user","id":"` + editorID + `","name":"Ada","origin":"web","client":null,"run_id":"` + runID + `","label":"Observe Agent for Ada"}`,
		},
		{
			name: "mcp client with a known name",
			item: Item{Author: Author{Kind: "user", ID: editorID, Name: "Ada"}, Origin: "mcp", OriginClient: "claude-code"},
			want: `{"kind":"user","id":"` + editorID + `","name":"Ada","origin":"mcp","client":"Claude Code","run_id":null,"label":"Ada (via Claude Code)"}`,
		},
		{
			name: "mcp client that sent no name",
			item: Item{Author: Author{Kind: "service_account", ID: otherID, Name: "ci"}, Origin: "mcp"},
			want: `{"kind":"service_account","id":"` + otherID + `","name":"ci","origin":"mcp","client":null,"run_id":null,"label":"ci (via MCP)"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := json.Marshal(NewSummaryJSON(c.item))
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Actor json.RawMessage `json:"actor"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if string(out.Actor) != c.want {
				t.Fatalf("actor = %s\nwant    %s", out.Actor, c.want)
			}
		})
	}
}

func TestProposalJSONKeepsTheAuthorFieldsBesideTheActor(t *testing.T) {
	item := Item{Author: Author{Kind: "user", ID: editorID, Name: "Ada"}, Origin: "mcp", OriginClient: "cursor-vscode"}
	raw, err := json.Marshal(NewProposalJSON(Proposal{Item: item}))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"author":        `{"kind":"user","id":"` + editorID + `","name":"Ada"}`,
		"origin":        `"mcp"`,
		"origin_client": `"cursor-vscode"`,
		"run_id":        `null`,
	}
	for key, value := range want {
		if string(out[key]) != value {
			t.Fatalf("%s = %s, want %s", key, out[key], value)
		}
	}
	if _, ok := out["actor"]; !ok {
		t.Fatal("actor is missing")
	}
}
