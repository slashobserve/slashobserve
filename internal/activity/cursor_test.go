package activity

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestCursorRoundTrip(t *testing.T) {
	for _, p := range []Position{{xid: 1, seq: 1}, {xid: 4294967296, seq: 9007199254740993}, {xid: 18446744073709551615, seq: 9223372036854775807}} {
		got, err := decodeCursor(encodeCursor(p))
		if err != nil {
			t.Fatalf("%+v: %v", p, err)
		}
		if got != p {
			t.Errorf("got %+v, want %+v", got, p)
		}
	}
}

func TestDecodeCursorRefusesBadValues(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := map[string]string{
		"empty":            "",
		"not base64":       "!!!",
		"no separator":     encode("12"),
		"zero xid":         encode("0.5"),
		"zero seq":         encode("5.0"),
		"negative xid":     encode("-5.5"),
		"negative seq":     encode("5.-5"),
		"not numbers":      encode("a.b"),
		"extra part":       encode("1.2.3"),
		"xid out of range": encode("18446744073709551616.1"),
		"padded base64":    encode("1.2") + "=",
	}
	for name, in := range tests {
		if _, err := decodeCursor(in); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%s: got %v, want ErrInvalidCursor", name, err)
		}
	}
}
