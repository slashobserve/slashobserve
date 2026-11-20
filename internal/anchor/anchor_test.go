package anchor_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/anchor"
)

type validCase struct {
	Name     string          `json:"name"`
	Anchor   json.RawMessage `json:"anchor"`
	Describe string          `json:"describe"`
}

type invalidCase struct {
	Name   string          `json:"name"`
	Anchor json.RawMessage `json:"anchor"`
	Error  string          `json:"error"`
}

type sharedCases struct {
	Valid   []validCase   `json:"valid"`
	Invalid []invalidCase `json:"invalid"`
}

func readCases(t *testing.T) sharedCases {
	t.Helper()
	root, err := os.OpenRoot("../../spec/examples")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile("anchor/anchor-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases sharedCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestSharedCasesCoverEveryKind(t *testing.T) {
	cases := readCases(t)
	seen := map[anchor.Kind]bool{}
	for _, c := range cases.Valid {
		a, err := anchor.Parse(string(c.Anchor))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		seen[a.Kind] = true
	}
	for _, k := range allKinds {
		if !seen[k] {
			t.Errorf("no valid shared case for kind %s", k)
		}
	}
}

var allKinds = []anchor.Kind{
	anchor.Time, anchor.Range, anchor.Series, anchor.Point, anchor.Row, anchor.Column, anchor.Cell,
	anchor.Bar, anchor.Segment, anchor.HeatCell, anchor.Slice, anchor.Value, anchor.LogLine,
	anchor.Span, anchor.Element, anchor.Text, anchor.Title, anchor.Variable, anchor.Position,
}

func TestDescribeSharedCases(t *testing.T) {
	for _, c := range readCases(t).Valid {
		t.Run(c.Name, func(t *testing.T) {
			a, err := anchor.Parse(string(c.Anchor))
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := a.Describe(); got != c.Describe {
				t.Errorf("Describe() = %q, want %q", got, c.Describe)
			}
		})
	}
}

func TestParseRejectsSharedInvalidCases(t *testing.T) {
	cases := readCases(t).Invalid
	if len(cases) < 30 {
		t.Fatalf("%d invalid cases, want at least 30", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			_, err := anchor.Parse(string(c.Anchor))
			if err == nil {
				t.Fatal("Parse accepted it")
			}
			if err.Error() != c.Error {
				t.Errorf("error = %q, want %q", err.Error(), c.Error)
			}
		})
	}
}

func TestParseRejectsMalformedText(t *testing.T) {
	for name, text := range map[string]string{
		"not json":          `row:service=payments`,
		"empty":             ``,
		"unknown field":     `{"kind":"title","pos":{"x":0,"y":0},"colour":"red"}`,
		"trailing data":     `{"kind":"title","pos":{"x":0,"y":0}} {}`,
		"bad time":          `{"kind":"time","pos":{"x":0,"y":0},"from":"yesterday"}`,
		"string pos":        `{"kind":"title","pos":"0.4,0.3"}`,
		"array":             `[]`,
		"null":              `null`,
		"unknown in pos":    `{"kind":"title","pos":{"x":0,"y":0,"z":1}}`,
		"unknown in window": `{"kind":"title","pos":{"x":0,"y":0},"window":{"from":"a","to":"b","tz":"utc"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := anchor.Parse(text); err == nil {
				t.Fatal("Parse accepted it")
			}
		})
	}
}

func TestMarshalRoundTripsEveryValidSharedCase(t *testing.T) {
	for _, c := range readCases(t).Valid {
		t.Run(c.Name, func(t *testing.T) {
			first, err := anchor.Parse(string(c.Anchor))
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			second, err := anchor.Parse(string(data))
			if err != nil {
				t.Fatalf("reparse %s: %v", data, err)
			}
			again, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != string(data) {
				t.Errorf("second marshal %s differs from first %s", again, data)
			}
			if second.Describe() != first.Describe() {
				t.Errorf("Describe changed: %q then %q", first.Describe(), second.Describe())
			}
		})
	}
}

func TestMarshalUsesTheWireNames(t *testing.T) {
	from := time.Date(2026, 11, 20, 23, 56, 0, 0, time.UTC)
	data, err := json.Marshal(anchor.Anchor{
		Kind: anchor.HeatCell, Pos: anchor.Pos{X: 0.25, Y: 0.75}, From: &from, Category: "100ms",
		Window: &anchor.Window{From: "now-1h", To: "now"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"heat_cell","pos":{"x":0.25,"y":0.75},"from":"2026-11-20T23:56:00Z","category":"100ms","window":{"from":"now-1h","to":"now"}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestValidateOnATypedValue(t *testing.T) {
	if err := (anchor.Anchor{Kind: anchor.Point, Series: map[string]string{"a": "b"}}).Validate(); err == nil || !strings.Contains(err.Error(), "point needs from") {
		t.Errorf("got %v", err)
	}
	if err := (anchor.Anchor{Kind: anchor.Title, Pos: anchor.Pos{X: 0.5, Y: 0.5}}).Validate(); err != nil {
		t.Errorf("got %v", err)
	}
	if err := (anchor.Anchor{Kind: anchor.Title, Pos: anchor.Pos{X: nan(), Y: 0.5}}).Validate(); err == nil {
		t.Error("NaN pos was accepted")
	}
}

func nan() float64 {
	zero := 0.0
	return zero / zero
}
