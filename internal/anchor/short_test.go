package anchor

import (
	"strings"
	"testing"
	"time"
)

var shortNow = time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)

func at(day, hour, minute int) *time.Time {
	t := time.Date(2026, 11, day, hour, minute, 0, 0, time.UTC)
	return &t
}

func TestParseShortFormsBuildTheAnchor(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want Anchor
	}{
		{"row", "row:service=payments,span=authorize_card", Anchor{Kind: Row, Pos: centre, Row: map[string]string{"service": "payments", "span": "authorize_card"}}},
		{"series", "series:service=checkout", Anchor{Kind: Series, Pos: centre, Series: map[string]string{"service": "checkout"}}},
		{"label value with an equals sign", "series:query=a=b", Anchor{Kind: Series, Pos: centre, Series: map[string]string{"query": "a=b"}}},
		{"time", "time:2026-11-20T23:56Z", Anchor{Kind: Time, Pos: centre, From: at(20, 23, 56)}},
		{"time with an offset", "time:2026-11-20T23:56+02:00", Anchor{Kind: Time, Pos: centre, From: new(time.Date(2026, 11, 20, 23, 56, 0, 0, time.FixedZone("", 7200)))}},
		{"time of day later than now is yesterday", "time:23:56Z", Anchor{Kind: Time, Pos: centre, From: at(20, 23, 56)}},
		{"time of day earlier than now is today", "time:08:15Z", Anchor{Kind: Time, Pos: centre, From: at(21, 8, 15)}},
		{"range of instants", "range:2026-11-20T23:50Z..2026-11-20T23:58Z", Anchor{Kind: Range, Pos: centre, From: at(20, 23, 50), To: at(20, 23, 58)}},
		{"range of times of day", "range:23:50Z..23:58Z", Anchor{Kind: Range, Pos: centre, From: at(20, 23, 50), To: at(20, 23, 58)}},
		{"range of times of day across midnight", "range:23:50Z..00:10Z", Anchor{Kind: Range, Pos: centre, From: at(20, 23, 50), To: at(21, 0, 10)}},
		{"range from a time of day to an instant", "range:23:50Z..2026-11-19T23:58Z", Anchor{Kind: Range, Pos: centre, From: at(19, 23, 50), To: at(19, 23, 58)}},
		{"range from an instant to a time of day", "range:2026-11-20T23:50Z..23:58Z", Anchor{Kind: Range, Pos: centre, From: at(20, 23, 50), To: at(20, 23, 58)}},
		{"pos", "pos:0.4,0.3", Anchor{Kind: Position, Pos: Pos{X: 0.4, Y: 0.3}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseShort(c.text, shortNow)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != c.want.Kind || got.Pos != c.want.Pos {
				t.Fatalf("kind and pos = %s %v, want %s %v", got.Kind, got.Pos, c.want.Kind, c.want.Pos)
			}
			if !sameTime(got.From, c.want.From) || !sameTime(got.To, c.want.To) {
				t.Fatalf("from and to = %v %v, want %v %v", got.From, got.To, c.want.From, c.want.To)
			}
			if !sameLabels(got.Row, c.want.Row) || !sameLabels(got.Series, c.want.Series) {
				t.Fatalf("labels = %v %v, want %v %v", got.Row, got.Series, c.want.Row, c.want.Series)
			}
		})
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sameLabels(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestParseShortRefusesWhatItCannotRead(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want string
	}{
		{"no form", "payments", "unrecognised anchor"},
		{"unknown form", "cell:a=b", "unrecognised anchor"},
		{"label without a value", "row:service", `"service" is not label=value`},
		{"label without a name", "row:=payments", `"=payments" is not label=value`},
		{"repeated label", "series:a=1,a=2", `label "a" is given twice`},
		{"empty labels", "row:", "is not label=value"},
		{"bad time", "time:yesterday", "is not a time"},
		{"range without two ends", "range:23:50Z", "want <instant>..<instant>"},
		{"range with a bad end", "range:23:50Z..late", `"late" is not a time`},
		{"range ending before it starts", "range:2026-11-20T23:58Z..2026-11-20T23:50Z", "range ends before it starts"},
		{"pos with one number", "pos:0.4", "want <x>,<y>"},
		{"pos with a word", "pos:left,top", "want <x>,<y>"},
		{"pos outside the panel", "pos:1.4,0.3", "pos: x and y must be between 0 and 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseShort(c.text, shortNow)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want one containing %q", err, c.want)
			}
		})
	}
}

func TestParseTakesTheShortFormAndTheJSONObject(t *testing.T) {
	short, err := Parse("pos:0.4,0.3")
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(`{"kind":"position","pos":{"x":0.4,"y":0.3}}`)
	if err != nil {
		t.Fatal(err)
	}
	if short.Kind != object.Kind || short.Pos != object.Pos {
		t.Fatalf("short %v differs from object %v", short, object)
	}
	if _, err := Parse("pos:0.4"); err == nil || !strings.HasPrefix(err.Error(), "anchor: ") {
		t.Fatalf("error = %v, want one prefixed with anchor:", err)
	}
}
