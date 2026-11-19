package usertext

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Reason string

const (
	ReasonEmpty         Reason = "text_empty"
	ReasonInvisibleOnly Reason = "text_invisible_only"
	ReasonInvisible     Reason = "text_invisible_characters"
	ReasonControl       Reason = "text_control_characters"
	ReasonBidi          Reason = "text_bidi_override"
	ReasonTooLong       Reason = "text_too_long"
	ReasonNotUTF8       Reason = "text_invalid_utf8"
)

type Rule struct {
	Max   int
	Lines bool
}

func Line(runes int) Rule { return Rule{Max: runes} }

func Text(runes int) Rule { return Rule{Max: runes, Lines: true} }

type Error struct {
	Field  string
	Reason Reason
	Rule   Rule
}

func (e *Error) Error() string {
	switch e.Reason {
	case ReasonEmpty:
		return fmt.Sprintf("%s must not be empty; enter 1 to %d characters", e.Field, e.Rule.Max)
	case ReasonInvisibleOnly:
		return fmt.Sprintf("%s has no visible character; enter 1 to %d characters of visible text", e.Field, e.Rule.Max)
	case ReasonInvisible:
		return fmt.Sprintf("%s must not contain zero-width or hidden characters; remove them", e.Field)
	case ReasonControl:
		if e.Rule.Lines {
			return fmt.Sprintf("%s must not contain control characters other than line breaks and tabs; remove them", e.Field)
		}
		return fmt.Sprintf("%s must be one line without control characters; remove line breaks, tabs and other control characters", e.Field)
	case ReasonBidi:
		return fmt.Sprintf("%s must not contain bidirectional override characters (U+202A to U+202E, U+2066 to U+2069); remove them", e.Field)
	case ReasonTooLong:
		return fmt.Sprintf("%s must be at most %d characters; shorten it", e.Field, e.Rule.Max)
	default:
		return fmt.Sprintf("%s must be valid UTF-8", e.Field)
	}
}

func ReasonOf(err error) (Reason, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason, true
	}
	return "", false
}

func Clean(field, raw string, rule Rule) (string, error) {
	trimmed := strings.TrimFunc(raw, edge)
	if trimmed == "" {
		err := Check(field, raw, rule)
		if err == nil {
			err = &Error{Field: field, Reason: ReasonEmpty, Rule: rule}
		}
		return "", err
	}
	if err := Check(field, trimmed, rule); err != nil {
		return "", err
	}
	return trimmed, nil
}

func Check(field, raw string, rule Rule) error {
	fail := func(reason Reason) error { return &Error{Field: field, Reason: reason, Rule: rule} }
	if !utf8.ValidString(raw) {
		return fail(ReasonNotUTF8)
	}
	if strings.IndexFunc(raw, func(r rune) bool { return !invisible(r) }) < 0 {
		if strings.IndexFunc(raw, func(r rune) bool { return !unicode.IsSpace(r) }) < 0 {
			return fail(ReasonEmpty)
		}
		return fail(ReasonInvisibleOnly)
	}
	inFlag := false
	for _, r := range raw {
		switch {
		case control(r, rule):
			return fail(ReasonControl)
		case bidiOverride(r):
			return fail(ReasonBidi)
		case tag(r) && !inFlag, !rule.Lines && zeroWidth(r):
			return fail(ReasonInvisible)
		}
		inFlag = r == flagBase || inFlag && tag(r)
	}
	if utf8.RuneCountInString(raw) > rule.Max {
		return fail(ReasonTooLong)
	}
	return nil
}

func control(r rune, rule Rule) bool {
	if rule.Lines && (r == '\n' || r == '\r' || r == '\t') {
		return false
	}
	return unicode.IsControl(r)
}

func bidiOverride(r rune) bool {
	return r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069'
}

func invisible(r rune) bool {
	return unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || filler(r)
}

func edge(r rune) bool {
	return unicode.IsSpace(r) || zeroWidth(r) || r == '\u200c' || r == '\u200d'
}

func filler(r rune) bool {
	switch r {
	case '\u034f', '\u115f', '\u1160', '\u2800', '\u3164', '\uffa0':
		return true
	}
	return false
}

const flagBase = '\U0001F3F4'

func tag(r rune) bool { return r >= 0xe0000 && r <= 0xe007f }

func zeroWidth(r rune) bool {
	switch {
	case r == '\u200b', r == '\ufeff', r == '\u00ad', r == '\u180e':
		return true
	case r >= '\u2060' && r <= '\u2064', r >= '\u206a' && r <= '\u206f':
		return true
	}
	return filler(r)
}
