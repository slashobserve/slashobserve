package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestDecodeTimeCursorRejectsMalformedValues(t *testing.T) {
	const id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	cases := map[string]string{
		"not base64":            "!!!",
		"no separator":          encode("123"),
		"empty micros":          encode(":" + id),
		"negative micros":       encode("-1:" + id),
		"micros past year 9999": encode("253402300800000000:" + id),
		"overflowing micros":    encode("99999999999999999999:" + id),
		"padded micros":         encode("0123:" + id),
		"signed micros":         encode("+123:" + id),
		"short id":              encode("123:" + id[1:]),
		"dashless id":           encode("123:" + strings.ReplaceAll(id, "-", "")),
		"misplaced dashes":      encode("123:aaaaaaaabbbb-cccc-dddd-eeeeeeeeeeee-"),
		"trailing text":         encode("123:" + id + ":x"),
	}
	for name, cursor := range cases {
		if _, ok := DecodeTimeCursor(cursor); ok {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTimeCursorRoundTrip(t *testing.T) {
	const id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	created := time.Date(2026, 10, 18, 12, 30, 45, 123456000, time.UTC)
	got, ok := DecodeTimeCursor(EncodeTimeCursor(created, id))
	if !ok {
		t.Fatal("round trip was rejected")
	}
	if !got.Time.Equal(created) || got.ID.String() != id {
		t.Errorf("round trip gave %v %v, want %v %v", got.Time, got.ID, created, id)
	}
	if empty, ok := DecodeTimeCursor(""); !ok || empty != nil {
		t.Errorf("empty cursor gave %v, %v", empty, ok)
	}
}
