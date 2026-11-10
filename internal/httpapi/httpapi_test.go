package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func TestOffsetParam(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		want   int
		status int
	}{
		{"absent", "", 0, http.StatusOK},
		{"zero", "offset=0", 0, http.StatusOK},
		{"fifty", "offset=50", 50, http.StatusOK},
		{"at the ceiling", "offset=100", 100, http.StatusOK},
		{"over the ceiling", "offset=101", 0, http.StatusBadRequest},
		{"negative", "offset=-1", 0, http.StatusBadRequest},
		{"leading zero", "offset=01", 0, http.StatusBadRequest},
		{"plus sign", "offset=%2B5", 0, http.StatusBadRequest},
		{"fraction", "offset=1.5", 0, http.StatusBadRequest},
		{"letters", "offset=abc", 0, http.StatusBadRequest},
		{"empty", "offset=", 0, http.StatusBadRequest},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil)
			got, ok := OffsetParam(c, 100)
			if (tc.status == http.StatusOK) != ok {
				t.Fatalf("ok = %v, want status %d", ok, tc.status)
			}
			if ok && got != tc.want {
				t.Fatalf("offset = %d, want %d", got, tc.want)
			}
			if !ok && rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestNonEmptyQuery(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		want   string
		status int
	}{
		{"absent", "", "", http.StatusOK},
		{"present", "status=open", "open", http.StatusOK},
		{"other parameter only", "panel_id=rps", "", http.StatusOK},
		{"empty", "status=", "", http.StatusBadRequest},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil)
			got, ok := NonEmptyQuery(c, "status")
			if (tc.status == http.StatusOK) != ok {
				t.Fatalf("ok = %v, want status %d", ok, tc.status)
			}
			if ok && got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
			if !ok && rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}
