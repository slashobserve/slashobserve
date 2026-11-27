package grounding

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const (
	renderTolerance = 0.01
	UnreadMarker    = "[figure not read]"
	SampledLead     = "at least "
)

var placeholder = regexp.MustCompile(`\{\{\s*(f\d+)((?:\s*:\s*[A-Za-z0-9%]+)*)\s*\}\}`)

type Problem struct {
	Kind string
	Text string
	Fix  string
}

type Ledger struct {
	Facts []Fact
}

func (r *Record) Ledger() *Ledger {
	if r == nil {
		return nil
	}
	return &Ledger{Facts: slices.Clone(r.Facts)}
}

func (l *Ledger) fact(id string) (Fact, bool) {
	if l == nil {
		return Fact{}, false
	}
	i := slices.IndexFunc(l.Facts, func(f Fact) bool { return f.ID == id })
	if i < 0 {
		return Fact{}, false
	}
	return l.Facts[i], true
}

func (l *Ledger) QueryOf(id string) string {
	f, _ := l.fact(id)
	return f.Query
}

type citation struct {
	text    string
	id      string
	options []string
}

func citations(text string) []citation {
	var out []citation
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		c := citation{text: m[0], id: m[1]}
		for _, opt := range strings.Split(m[2], ":") {
			if opt = strings.TrimSpace(opt); opt != "" {
				c.options = append(c.options, opt)
			}
		}
		out = append(out, c)
	}
	return out
}

func (l *Ledger) Render(text string) (string, []Problem) {
	var problems []Problem
	out := placeholder.ReplaceAllStringFunc(text, func(match string) string {
		c := citations(match)[0]
		f, ok := l.fact(c.id)
		if !ok {
			problems = append(problems, Problem{Kind: findings.UnverifiedFact, Text: c.text, Fix: c.id + " is not a fact this run read, or it is no longer held: read the value again and cite the id the result gives"})
			return UnreadMarker
		}
		shown, err := Format(f, c.options)
		if err != nil {
			problems = append(problems, Problem{Kind: findings.UnverifiedFact, Text: c.text, Fix: err.Error()})
			shown, _ = Format(f, nil)
		}
		return shown
	})
	return out, problems
}

var durationScales = map[string]float64{"us": 1e-6, "ms": 1e-3, "s": 1, "min": 60}

var unitText = map[string]string{"us": " µs", "ms": " ms", "s": " s", "min": " min", "%": "%", "bytes": " bytes"}

type rendering struct {
	value  float64
	unit   string
	digits int
	bare   bool
	moved  bool
}

func Format(f Fact, options []string) (string, error) {
	if f.Kind == FactTraceID {
		if len(options) > 0 {
			return "", fmt.Errorf("%s is a trace id, which takes no options; cite it as {{%s}}", f.ID, f.ID)
		}
		return f.Text, nil
	}
	r := rendering{value: f.Value, unit: f.Unit}
	for _, opt := range options {
		if err := r.apply(f, opt); err != nil {
			return "", err
		}
	}
	text := f.Text
	if r.moved || r.digits > 0 {
		shown := r.value
		if r.digits > 0 {
			shown = significant(r.value, r.digits)
			if off := relative(shown, r.value); off > renderTolerance {
				return "", fmt.Errorf("{{%s:%d}} would show %s, %.1f%% away from %s; give more significant figures", f.ID, r.digits, plain(shown), off*100, plain(r.value))
			}
			if f.Sampled && shown > r.value {
				return "", fmt.Errorf("{{%s:%d}} would show at least %s, above the %s counted in a sample; give more significant figures", f.ID, r.digits, plain(shown), plain(r.value))
			}
		}
		text = plain(shown)
	}
	if !r.bare {
		text += unitText[r.unit]
	}
	if f.Sampled {
		text = SampledLead + text
	}
	return text, nil
}

func (r *rendering) apply(f Fact, opt string) error {
	switch {
	case opt == "bare":
		r.bare = true
	case opt == "%":
		if f.Kind != FactRatio && f.Kind != FactNumber || r.unit != "" {
			return fmt.Errorf("%s is a %s, so it cannot be shown as a percent", f.ID, describeKind(f))
		}
		r.value, r.unit, r.moved = r.value*100, "%", true
	case durationScales[opt] != 0:
		return r.convert(f, opt)
	case len(opt) == 1 && opt[0] >= '1' && opt[0] <= '9':
		r.digits = int(opt[0] - '0')
	default:
		return fmt.Errorf("%q is not an option for %s; use ms, s, us, min, %%, bare or 1 to 9 significant figures", opt, f.ID)
	}
	return nil
}

func (r *rendering) convert(f Fact, opt string) error {
	from := r.unit
	if from == "" && f.Kind == FactNumber {
		if from = namedUnit(f); from == "" {
			return fmt.Errorf("%s is a number whose unit the read did not name, so it cannot be converted to %s; cite it as {{%s}} and write the unit its query measures after it", f.ID, opt, f.ID)
		}
	}
	scale, ok := durationScales[from]
	if !ok || f.Kind != FactDuration && f.Kind != FactNumber {
		return fmt.Errorf("%s is a %s, so it cannot be shown in %s", f.ID, describeKind(f), opt)
	}
	r.value, r.unit, r.moved = r.value*scale/durationScales[opt], opt, true
	return nil
}

var namedUnits = []struct {
	match *regexp.Regexp
	unit  string
}{
	{regexp.MustCompile(`(?i)(^|[^a-z0-9])(ms|millis|milliseconds)([^a-z0-9]|$)`), "ms"},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9])(micros|microseconds)([^a-z0-9]|$)`), "us"},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9])(sec|secs|seconds)([^a-z0-9]|$)`), "s"},
}

func namedUnit(f Fact) string {
	found := ""
	for _, text := range []string{f.Of, f.Query, f.Asked} {
		for _, n := range namedUnits {
			if !n.match.MatchString(text) {
				continue
			}
			if found != "" && found != n.unit {
				return ""
			}
			found = n.unit
		}
		if found != "" {
			return found
		}
	}
	return ""
}

func describeKind(f Fact) string {
	if f.Unit != "" {
		return string(f.Kind) + " in " + f.Unit
	}
	return string(f.Kind)
}

func significant(v float64, digits int) float64 {
	out, err := strconv.ParseFloat(strconv.FormatFloat(v, 'g', digits, 64), 64)
	if err != nil {
		return v
	}
	return out
}

func plain(v float64) string {
	return strconv.FormatFloat(significant(v, 12), 'f', -1, 64)
}

func relative(shown, value float64) float64 {
	if value == 0 {
		return math.Abs(shown)
	}
	return math.Abs(shown-value) / math.Abs(value)
}
