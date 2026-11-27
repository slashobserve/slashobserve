package public

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLinkJSONStateFollowsExpiredAndLeavesTheFlagsAlone(t *testing.T) {
	tests := []struct {
		name string
		link Link
		want string
	}{
		{"live link", Link{Usable: true}, "active"},
		{"expired link", Link{Expired: true}, "expired"},
		{"unusable but not expired", Link{}, "active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkJSONOf(tt.link)
			if got.State != tt.want || got.Expired != tt.link.Expired || got.Usable != tt.link.Usable {
				t.Errorf("linkJSONOf = state %s expired %v usable %v, want state %s", got.State, got.Expired, got.Usable, tt.want)
			}
		})
	}
}

func TestLinkJSONSaysWhyALinkIsNotUsable(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Second)
	tests := []struct {
		name string
		link Link
		want string
	}{
		{"a link that opens", judged(Link{}, false, "", now), `null`},
		{"a link past its expiry", judged(Link{ExpiresAt: &past}, false, "", now), `"expired"`},
		{"a link whose dashboard sits in a folder only Admins can read", judged(Link{}, true, "", now), `"admins_only_folder"`},
		{"a link whose dashboard reads the viewer's identity", judged(Link{}, false, reasonUserScoped, now), `"user_scoped"`},
		{"a link whose dashboard reads a datasource only Admins can query", judged(Link{}, false, reasonRestrictedDatasource, now), `"restricted_datasource"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(linkJSONOf(tt.link))
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]json.RawMessage
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if got := string(out["unusable_reason"]); got != tt.want {
				t.Fatalf("unusable_reason = %q, want %s", got, tt.want)
			}
			if usable := string(out["usable"]); (usable == "true") != (tt.want == "null") {
				t.Fatalf("usable = %s beside unusable_reason %s", usable, tt.want)
			}
		})
	}
}
