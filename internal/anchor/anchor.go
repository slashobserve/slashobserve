package anchor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/usertext"
)

type Kind string

const (
	Time     Kind = "time"
	Range    Kind = "range"
	Series   Kind = "series"
	Point    Kind = "point"
	Row      Kind = "row"
	Column   Kind = "column"
	Cell     Kind = "cell"
	Bar      Kind = "bar"
	Segment  Kind = "segment"
	HeatCell Kind = "heat_cell"
	Slice    Kind = "slice"
	Value    Kind = "value"
	LogLine  Kind = "log_line"
	Span     Kind = "span"
	Element  Kind = "element"
	Text     Kind = "text"
	Title    Kind = "title"
	Variable Kind = "variable"
	Position Kind = "position"
)

func KindOf(raw []byte) Kind {
	var a struct {
		Kind Kind `json:"kind"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return ""
	}
	return a.Kind
}

type Pos struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Window struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Anchor struct {
	Kind     Kind              `json:"kind"`
	Pos      Pos               `json:"pos"`
	From     *time.Time        `json:"from,omitempty"`
	To       *time.Time        `json:"to,omitempty"`
	Series   map[string]string `json:"series,omitempty"`
	Row      map[string]string `json:"row,omitempty"`
	Column   string            `json:"column,omitempty"`
	Category string            `json:"category,omitempty"`
	ID       string            `json:"id,omitempty"`
	Quote    string            `json:"quote,omitempty"`
	Shown    string            `json:"shown,omitempty"`
	Window   *Window           `json:"window,omitempty"`
}

const (
	maxLabels   = 16
	maxText     = 200
	maxQuote    = 280
	maxEncoded  = 1536
	fieldFrom   = "from"
	fieldTo     = "to"
	fieldSeries = "series"
	fieldRow    = "row"
	fieldColumn = "column"
	fieldCat    = "category"
	fieldID     = "id"
	fieldQuote  = "quote"
)

var fieldOrder = []string{fieldFrom, fieldTo, fieldSeries, fieldRow, fieldColumn, fieldCat, fieldID, fieldQuote}

type fieldRule struct {
	required []string
	optional []string
}

var kindRules = map[Kind]fieldRule{
	Time:     {required: []string{fieldFrom}},
	Range:    {required: []string{fieldFrom, fieldTo}},
	Series:   {required: []string{fieldSeries}},
	Point:    {required: []string{fieldSeries, fieldFrom}},
	Row:      {required: []string{fieldRow}, optional: []string{fieldQuote}},
	Column:   {required: []string{fieldColumn}},
	Cell:     {required: []string{fieldRow, fieldColumn}},
	Bar:      {required: []string{fieldCat}, optional: []string{fieldSeries}},
	Segment:  {required: []string{fieldCat, fieldSeries}},
	HeatCell: {required: []string{fieldFrom, fieldCat}},
	Slice:    {required: []string{fieldCat}},
	Value:    {optional: []string{fieldSeries}},
	LogLine:  {required: []string{fieldID}, optional: []string{fieldFrom, fieldQuote, fieldSeries}},
	Span:     {required: []string{fieldID}, optional: []string{fieldQuote}},
	Element:  {required: []string{fieldID}},
	Text:     {required: []string{fieldQuote}},
	Title:    {},
	Variable: {required: []string{fieldID}},
	Position: {},
}

func (a Anchor) present(field string) bool {
	switch field {
	case fieldFrom:
		return a.From != nil
	case fieldTo:
		return a.To != nil
	case fieldSeries:
		return len(a.Series) > 0
	case fieldRow:
		return len(a.Row) > 0
	case fieldColumn:
		return a.Column != ""
	case fieldCat:
		return a.Category != ""
	case fieldID:
		return a.ID != ""
	case fieldQuote:
		return a.Quote != ""
	}
	return false
}

func (a Anchor) Validate() error {
	rule, ok := kindRules[a.Kind]
	if !ok {
		return fmt.Errorf("unknown kind %q", a.Kind)
	}
	if !inUnit(a.Pos.X) || !inUnit(a.Pos.Y) {
		return errors.New("pos: x and y must be between 0 and 1")
	}
	for _, field := range rule.required {
		if !a.present(field) {
			return fmt.Errorf("%s needs %s", a.Kind, field)
		}
	}
	for _, field := range fieldOrder {
		if a.present(field) && !slices.Contains(rule.required, field) && !slices.Contains(rule.optional, field) {
			return fmt.Errorf("%s does not take %s", a.Kind, field)
		}
	}
	if a.Kind == Range && a.To.Before(*a.From) {
		return errors.New("range ends before it starts")
	}
	if err := a.validateSizes(); err != nil {
		return err
	}
	return a.validateEncoded()
}

func (a Anchor) validateEncoded() error {
	raw, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if len(raw) > maxEncoded {
		return fmt.Errorf("%d bytes once encoded, over the limit of %d bytes; send fewer or shorter labels", len(raw), maxEncoded)
	}
	return nil
}

var notOnABlock = []Kind{Variable}

func (a Anchor) CheckOnBlock() error {
	if slices.Contains(notOnABlock, a.Kind) {
		return fmt.Errorf("block_id with an anchor of kind %q: a block has no %s; send the anchor without block_id, or block_id without an anchor", a.Kind, a.Kind)
	}
	return nil
}

func inUnit(v float64) bool {
	return !math.IsNaN(v) && v >= 0 && v <= 1
}

func (a Anchor) validateSizes() error {
	if err := validateLabels(fieldSeries, a.Series); err != nil {
		return err
	}
	if err := validateLabels(fieldRow, a.Row); err != nil {
		return err
	}
	texts := []struct {
		name  string
		value string
		limit int
	}{
		{fieldColumn, a.Column, maxText},
		{fieldCat, a.Category, maxText},
		{fieldID, a.ID, maxText},
		{fieldQuote, a.Quote, maxQuote},
		{"shown", a.Shown, maxText},
	}
	for _, t := range texts {
		if utf8.RuneCountInString(t.value) > t.limit {
			return fmt.Errorf("%s is longer than %d characters", t.name, t.limit)
		}
		if err := checkText(t.name, t.value, t.limit); err != nil {
			return err
		}
	}
	if a.Window != nil {
		if a.Window.From == "" || a.Window.To == "" {
			return errors.New("window needs from and to")
		}
		if utf8.RuneCountInString(a.Window.From) > maxText || utf8.RuneCountInString(a.Window.To) > maxText {
			return fmt.Errorf("window is longer than %d characters", maxText)
		}
		if err := checkText("window from", a.Window.From, maxText); err != nil {
			return err
		}
		return checkText("window to", a.Window.To, maxText)
	}
	return nil
}

func checkText(name, value string, limit int) error {
	err := usertext.Check(name, value, usertext.Text(limit))
	if reason, ok := usertext.ReasonOf(err); ok && reason == usertext.ReasonEmpty {
		return nil
	}
	return err
}

func validateLabels(name string, labels map[string]string) error {
	if len(labels) > maxLabels {
		return fmt.Errorf("%s has more than %d labels", name, maxLabels)
	}
	for k, v := range labels {
		if k == "" {
			return fmt.Errorf("%s has an empty label name", name)
		}
		if utf8.RuneCountInString(k) > maxText || utf8.RuneCountInString(v) > maxText {
			return fmt.Errorf("%s label is longer than %d characters", name, maxText)
		}
		if err := checkText(name+" label name", k, maxText); err != nil {
			return err
		}
		if err := checkText(name+" label value", v, maxText); err != nil {
			return err
		}
	}
	return nil
}

func (a *Anchor) UnmarshalJSON(data []byte) error {
	type plain Anchor
	wire := struct {
		plain
		Pos *Pos `json:"pos"`
	}{}
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if wire.Pos == nil {
		return errors.New("pos is required")
	}
	*a = Anchor(wire.plain)
	a.Pos = *wire.Pos
	return nil
}

func decodeStrict(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the anchor")
	}
	return nil
}

func Parse(text string) (Anchor, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "{") {
		a, err := parseShort(strings.TrimSpace(text), time.Now())
		if err != nil {
			return Anchor{}, fmt.Errorf("anchor: %w", err)
		}
		return a, nil
	}
	var a Anchor
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return Anchor{}, fmt.Errorf("anchor: %w", err)
	}
	if err := a.Validate(); err != nil {
		return Anchor{}, fmt.Errorf("anchor: %w", err)
	}
	return a, nil
}

var relativeWindow = regexp.MustCompile(`^now-([1-9][0-9]*)([smhdw])$`)

var windowUnits = map[string]string{"s": "second", "m": "minute", "h": "hour", "d": "day", "w": "week"}

func describeWindow(w Window) string {
	if m := relativeWindow.FindStringSubmatch(w.From); m != nil && w.To == "now" {
		unit := windowUnits[m[2]]
		if m[1] == "1" {
			return "last " + unit
		}
		return "last " + m[1] + " " + unit + "s"
	}
	return w.From + " to " + w.To
}

func describeTime(t time.Time) string {
	t = t.UTC()
	layout := "2006-01-02 15:04"
	if t.Second() != 0 {
		layout = "2006-01-02 15:04:05"
	}
	return t.Format(layout) + " UTC"
}

func labelValues(labels map[string]string) string {
	keys := sortedKeys(labels)
	values := make([]string, len(keys))
	for i, k := range keys {
		values[i] = labels[k]
	}
	return strings.Join(values, ", ")
}

func labelPairs(labels map[string]string) string {
	keys := sortedKeys(labels)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + labels[k]
	}
	return strings.Join(pairs, ", ")
}

func sortedKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func percent(v float64) string {
	return strconv.Itoa(int(math.Round(v*100))) + "%"
}

func joinNonEmpty(parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

func (a Anchor) fromText() string {
	if a.From == nil {
		return ""
	}
	return describeTime(*a.From)
}

func (a Anchor) logLineSubject() string {
	at := a.fromText()
	if at != "" {
		at = "at " + at
	}
	in := ""
	if len(a.Series) > 0 {
		in = "in the series " + labelPairs(a.Series)
	}
	return joinNonEmpty("the log line "+a.ID, at, a.Quote, in)
}

var subjects = map[Kind]func(Anchor) string{
	Time:   func(a Anchor) string { return "the moment " + a.fromText() },
	Range:  func(a Anchor) string { return "the range " + a.fromText() + " to " + describeTime(*a.To) },
	Series: func(a Anchor) string { return "the series " + labelPairs(a.Series) },
	Point: func(a Anchor) string {
		return "the point at " + a.fromText() + " on the series " + labelPairs(a.Series)
	},
	Row:      func(a Anchor) string { return joinNonEmpty("the row "+labelValues(a.Row), a.Quote) },
	Column:   func(a Anchor) string { return "the column " + a.Column },
	Cell:     func(a Anchor) string { return "the cell " + a.Column + " in the row " + labelValues(a.Row) },
	Bar:      func(a Anchor) string { return "the bar " + a.Category + a.ofSeries() },
	Segment:  func(a Anchor) string { return "the segment " + a.Category + a.ofSeries() },
	HeatCell: func(a Anchor) string { return "the heat cell " + a.Category + " at " + a.fromText() },
	Slice:    func(a Anchor) string { return "the slice " + a.Category },
	Value:    func(a Anchor) string { return "the value" + a.ofSeries() },
	LogLine:  Anchor.logLineSubject,
	Span:     func(a Anchor) string { return joinNonEmpty("the span "+a.ID, a.Quote) },
	Element:  func(a Anchor) string { return "the element " + a.ID },
	Text:     func(a Anchor) string { return `the text "` + a.Quote + `"` },
	Title:    func(Anchor) string { return "the title" },
	Variable: func(a Anchor) string { return "the variable " + a.ID },
	Position: func(a Anchor) string { return "a spot at " + percent(a.Pos.X) + ", " + percent(a.Pos.Y) },
}

func (a Anchor) ofSeries() string {
	if len(a.Series) == 0 {
		return ""
	}
	return " of the series " + labelPairs(a.Series)
}

func (a Anchor) Describe() string {
	out := subjects[a.Kind](a)
	if a.Shown != "" {
		out += " (" + a.Shown + ")"
	}
	if a.Window != nil {
		out += ", " + describeWindow(*a.Window)
	}
	return out
}
