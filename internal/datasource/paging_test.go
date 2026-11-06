package datasource

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCursorRoundTripsThroughItsWireForm(t *testing.T) {
	scope := CursorScope("logs", `{service_name="checkout"}`)
	in := PageCursor{Scope: scope, Page: 3, FromNs: 1793962800000000000, ToNs: 1793966400000000005, EndNs: 1793966399000000001, Seen: []string{"a", "b"}}
	encoded := EncodeCursor(in)
	if strings.ContainsAny(encoded, "+/=") {
		t.Fatalf("cursor %q is not base64url without padding", encoded)
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !strings.HasPrefix(string(raw), `{"v":1,`) {
		t.Fatalf("cursor decodes to %q, %v; want version 1 JSON", raw, err)
	}
	out, err := DecodeCursor(encoded, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*out, in) {
		t.Fatalf("decoded = %+v, want %+v", *out, in)
	}
}

func TestCursorScopeIsSixteenHexOverTheDatasourceAndQuery(t *testing.T) {
	a := CursorScope("logs", `{a="b"}`)
	if len(a) != 16 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("scope = %q, want 16 hex characters", a)
	}
	if a == CursorScope("other", `{a="b"}`) || a == CursorScope("logs", `{a="c"}`) {
		t.Fatal("scope does not change with the datasource or the query")
	}
}

func TestDecodeCursorRefusesForeignAndBrokenCursors(t *testing.T) {
	scope := CursorScope("logs", "q")
	good := EncodeCursor(PageCursor{Scope: scope, Page: 2})
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"another query", EncodeCursor(PageCursor{Scope: CursorScope("logs", "other"), Page: 2}), "cursor does not belong to this query"},
		{"another datasource", EncodeCursor(PageCursor{Scope: CursorScope("traces", "q"), Page: 2}), "cursor does not belong to this query"},
		{"truncated", good[:len(good)/2], "cursor is not readable"},
		{"not base64", "not*a*cursor", "cursor is not readable"},
		{"oversize", strings.Repeat("A", MaxCursorBytes+1), "cursor is not readable"},
		{"page below one", EncodeCursor(PageCursor{Scope: scope, Page: 0}), "cursor is not readable"},
		{"other version", base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"scope":"` + scope + `","page":2}`)), "cursor is not readable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCursor(tc.input, scope)
			if !errors.Is(err, ErrInvalidQuery) || err.Error() != "invalid query: "+tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRowFingerprintIgnoresLabelOrderAndSeparatesLabelsFromTheLine(t *testing.T) {
	a := RowFingerprint(map[string]string{"service_name": "checkout", "level": "error"}, "payment failed")
	b := RowFingerprint(map[string]string{"level": "error", "service_name": "checkout"}, "payment failed")
	if a != b {
		t.Fatalf("fingerprints %q and %q differ by label order", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("fingerprint = %q, want 16 hex characters", a)
	}
	if a == RowFingerprint(map[string]string{"level": "error", "service_name": "checkout"}, "payment ok") {
		t.Fatal("fingerprint ignores the line")
	}
	if a == RowFingerprint(map[string]string{"level": "error", "service_name": "cart"}, "payment failed") {
		t.Fatal("fingerprint ignores the labels")
	}
}

func TestPageTotalCountsTheWalkedPages(t *testing.T) {
	cases := []struct {
		name    string
		page    int
		rows    int
		hasNext bool
		total   int64
		rel     TotalRelation
	}{
		{"first page with more", 1, 100, true, 100, TotalAtLeast},
		{"third page with more", 3, 100, true, 300, TotalAtLeast},
		{"last page", 3, 40, false, 240, TotalExact},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total, rel := PageTotal(tc.page, 100, tc.rows, tc.hasNext)
			if total != tc.total || rel != tc.rel {
				t.Fatalf("total = %d %s, want %d %s", total, rel, tc.total, tc.rel)
			}
		})
	}
}
