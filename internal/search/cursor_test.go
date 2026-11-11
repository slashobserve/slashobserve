package search

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrips(t *testing.T) {
	asOf := time.Date(2026, 11, 11, 10, 30, 15, 123_000_000, time.UTC)
	gotAsOf, gotOffset, err := decodeCursor(encodeCursor(asOf, 40))
	if err != nil {
		t.Fatal(err)
	}
	if !gotAsOf.Equal(asOf) || gotOffset != 40 {
		t.Fatalf("got %v and %d, want %v and 40", gotAsOf, gotOffset, asOf)
	}
}

func TestCursorRefusesWhatWeDidNotIssue(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	cases := map[string]string{
		"not base64":          "%%%",
		"not json":            encode("hello"),
		"empty object":        encode("{}"),
		"negative offset":     encode(`{"a":1763000000000,"o":-1}`),
		"offset at the cap":   encode(`{"a":1763000000000,"o":200}`),
		"offset beyond a cap": encode(`{"a":1763000000000,"o":5000}`),
		"no time":             encode(`{"o":20}`),
		"negative time":       encode(`{"a":-5,"o":20}`),
		"a wrong type":        encode(`{"a":"now","o":20}`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeCursor(raw); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("decodeCursor(%q) = %v, want ErrInvalidCursor", raw, err)
			}
		})
	}
}

func TestCursorAcceptsTheLastOffsetBelowTheCap(t *testing.T) {
	if _, _, err := decodeCursor(encodeCursor(time.Now(), MaxResults-1)); err != nil {
		t.Fatal(err)
	}
}
