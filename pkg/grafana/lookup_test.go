package grafana

import "testing"

func TestSplitIndexes(t *testing.T) {
	cases := []struct{ key, name, indexes string }{
		{"a", "a", ""},
		{"a[1]", "a", "[1]"},
		{"a[1][22]", "a", "[1][22]"},
		{"[3]", "", "[3]"},
		{"a[]", "a[]", ""},
		{"a[x]", "a[x]", ""},
		{"a[1", "a[1", ""},
		{"a1]", "a1]", ""},
		{"a[1]b", "a[1]b", ""},
		{"a[b][1]", "a[b]", "[1]"},
		{"a[[1]", "a[", "[1]"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if name, indexes := splitIndexes(tc.key); name != tc.name || indexes != tc.indexes {
			t.Errorf("splitIndexes(%q) = %q, %q, want %q, %q", tc.key, name, indexes, tc.name, tc.indexes)
		}
	}
}

func TestLookupFollowsKeysAndIndexes(t *testing.T) {
	doc := map[string]any{
		"a":    map[string]any{"b": []any{"x", map[string]any{"c": []any{[]any{"y", "z"}}}}},
		"d[1]": "literal",
	}
	cases := []struct {
		path string
		want any
		ok   bool
	}{
		{"a.b[0]", "x", true},
		{"a.b[1].c[0][1]", "z", true},
		{"a.b[2]", nil, false},
		{"a.b[99999999999999999999999]", nil, false},
		{"a.b[0].c", nil, false},
		{"a.missing", nil, false},
		{"a[0]", nil, false},
		{"d[1]", nil, false},
	}
	for _, tc := range cases {
		got, ok := lookup(doc, tc.path)
		if ok != tc.ok || got != tc.want {
			t.Errorf("lookup(%q) = %v, %v, want %v, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}
