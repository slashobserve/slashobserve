package usertext

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		rule   Rule
		want   string
		reason Reason
	}{
		{"trims spaces", "  QA lead  ", Line(100), "QA lead", ""},
		{"trims zero width at the ends", "\u200bQA\u200b ", Line(100), "QA", ""},
		{"empty", "", Line(100), "", ReasonEmpty},
		{"spaces only", "   ", Line(100), "", ReasonEmpty},
		{"zero width only", "\u200b", Line(100), "", ReasonInvisibleOnly},
		{"several fillers", "\u200b\u2060\ufeff\u3164", Text(100), "", ReasonInvisibleOnly},
		{"joiner only", "\u200d", Text(100), "", ReasonInvisibleOnly},
		{"newline in a line", "a\nb", Line(100), "", ReasonControl},
		{"bell", "a\x07b", Text(100), "", ReasonControl},
		{"nul", "a\x00b", Text(100), "", ReasonControl},
		{"newline in text", "a\nb\tc", Text(100), "a\nb\tc", ""},
		{"override", "QA-r2-\u202eevil", Line(100), "", ReasonBidi},
		{"isolate", "a\u2066b", Text(100), "", ReasonBidi},
		{"override at the edge is not trimmed away", "\u202eabc", Line(100), "", ReasonBidi},
		{"zero width inside a line", "QA\u200bx", Line(100), "", ReasonInvisible},
		{"zero width inside text", "QA\u200bx", Text(100), "QA\u200bx", ""},
		{"joiner inside an emoji sequence", "a \U0001F468\u200d\U0001F469", Line(100), "a \U0001F468\u200d\U0001F469", ""},
		{"tag characters", "a\U000E0041", Text(100), "", ReasonInvisible},
		{"england flag", "go \U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", Text(100), "go \U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", ""},
		{"england flag in a line", "\U0001F3F4\U000E0067\U000E0062\U000E007F", Line(100), "\U0001F3F4\U000E0067\U000E0062\U000E007F", ""},
		{"tag characters after a letter", "a\U000E0067\U000E007F", Text(100), "", ReasonInvisible},
		{"tag characters after a flag and a letter", "\U0001F3F4a\U000E0067", Text(100), "", ReasonInvisible},
		{"hebrew", "שלום", Line(100), "שלום", ""},
		{"counts characters not bytes", strings.Repeat("é", 100), Line(100), strings.Repeat("é", 100), ""},
		{"counts an emoji once", strings.Repeat("\U0001F600", 100), Line(100), strings.Repeat("\U0001F600", 100), ""},
		{"too long", strings.Repeat("a", 101), Line(100), "", ReasonTooLong},
		{"invalid utf8", "a\xffb", Line(100), "", ReasonNotUTF8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Clean("title", tc.raw, tc.rule)
			if tc.reason == "" {
				if err != nil || got != tc.want {
					t.Fatalf("Clean = %q, %v; want %q", got, err, tc.want)
				}
				return
			}
			reason, ok := ReasonOf(err)
			if !ok || reason != tc.reason {
				t.Fatalf("Clean error = %v; want reason %s", err, tc.reason)
			}
		})
	}
}

func TestCheckKeepsTheText(t *testing.T) {
	if err := Check("markdown", "    code\n", Text(100)); err != nil {
		t.Fatalf("Check = %v", err)
	}
	if reason, _ := ReasonOf(Check("markdown", "\u200b", Text(100))); reason != ReasonInvisibleOnly {
		t.Fatalf("reason = %s", reason)
	}
}

func TestErrorsSayWhatToChange(t *testing.T) {
	cases := map[Reason]string{
		ReasonEmpty:         "title must not be empty; enter 1 to 100 characters",
		ReasonInvisibleOnly: "title has no visible character; enter 1 to 100 characters of visible text",
		ReasonInvisible:     "title must not contain zero-width or hidden characters; remove them",
		ReasonControl:       "title must be one line without control characters; remove line breaks, tabs and other control characters",
		ReasonBidi:          "title must not contain bidirectional override characters (U+202A to U+202E, U+2066 to U+2069); remove them",
		ReasonTooLong:       "title must be at most 100 characters; shorten it",
		ReasonNotUTF8:       "title must be valid UTF-8",
	}
	for reason, want := range cases {
		if got := (&Error{Field: "title", Reason: reason, Rule: Line(100)}).Error(); got != want {
			t.Errorf("%s: %q, want %q", reason, got, want)
		}
	}
}
