package links

import (
	"net/url"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		{"words", "Checkout health", "checkout-health"},
		{"punctuation runs collapse", "Thanos / Query -- p99 (5m)", "thanos-query-p99-5m"},
		{"edges trimmed", "  --Shop!--  ", "shop"},
		{"digits kept", "K8s 1.30 nodes", "k8s-1-30-nodes"},
		{"accented letters separate", "Café Über ops", "caf-ber-ops"},
		{"non-latin only", "監視ダッシュボード", ""},
		{"dotted capital I is not folded", "İstanbul", "stanbul"},
		{"kelvin sign is not folded", "Kube", "ube"},
		{"emoji", "🔥 Errors 🔥", "errors"},
		{"empty", "", ""},
		{"only separators", "!!! ---", ""},
		{"history segment dropped", "Versions", ""},
		{"history segment as a prefix kept", "Versions v2", "versions-v2"},
		{"long title cut at 60", strings.Repeat("a", 70), strings.Repeat("a", 60)},
		{"cut lands on a separator", strings.Repeat("a", 59) + " bcd", strings.Repeat("a", 59)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slug(tc.title); got != tc.want {
				t.Fatalf("Slug(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestAddresses(t *testing.T) {
	public := &url.URL{Scheme: "https", Host: "observe.example", Path: "/team"}
	const id = "0b6f9c1e-7a52-4c1b-9d3e-2f8a6b4c5d10"
	cases := []struct {
		name, got, want string
	}{
		{"dashboard with a title", Dashboard(public, id, "Checkout health"), "https://observe.example/team/d/" + id + "/checkout-health"},
		{"dashboard without a title", Dashboard(public, id, ""), "https://observe.example/team/d/" + id},
		{"dashboard whose title has no slug", Dashboard(public, id, "監視"), "https://observe.example/team/d/" + id},
		{"relative dashboard", Dashboard(nil, id, "Shop"), "/d/" + id + "/shop"},
		{"relative id escaped", Dashboard(nil, "a/b", ""), "/d/a%2Fb"},
		{"folder", Folder(public, id), "https://observe.example/team/f/" + id},
		{"chat", Chat(public, id), "https://observe.example/team/c/" + id},
		{"build", Build(nil, id), "/b/" + id},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
