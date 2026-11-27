package activity

import (
	"encoding/json"
	"testing"
)

func inboxItemState(t *testing.T, item InboxItem) string {
	t.Helper()
	raw, err := json.Marshal(InboxJSONOf(InboxPage{Items: []InboxItem{item}}))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return string(out.Items[0]["state"])
}

func TestInboxItemJSONCarriesItsState(t *testing.T) {
	tests := []struct {
		name string
		item InboxItem
		want string
	}{
		{"an unread item", InboxItem{Unread: true}, `"unread"`},
		{"a read item", InboxItem{}, `"read"`},
		{"an item marked done", InboxItem{Done: true}, `"done"`},
		{"a done item whose unread flag is still set", InboxItem{Done: true, Unread: true}, `"done"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inboxItemState(t, tt.item); got != tt.want {
				t.Fatalf("state = %q, want %s", got, tt.want)
			}
		})
	}
}

func TestEveryInboxItemStateIsDeclared(t *testing.T) {
	for _, item := range []InboxItem{{}, {Unread: true}, {Done: true}} {
		if state := string(item.State()); !InboxItemStates.Has(state) {
			t.Errorf("InboxItemStates lacks %q", state)
		}
	}
}
