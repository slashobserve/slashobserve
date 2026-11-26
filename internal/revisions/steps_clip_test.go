package revisions

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAStepOverTheLimitEndsAtAWordBoundary(t *testing.T) {
	long := strings.Repeat("alpha beta gamma ", 30)
	got := cleanStepMessage(long, MaxStepMessageRunes)
	if utf8.RuneCountInString(got) > MaxStepMessageRunes || !strings.HasSuffix(got, "...") {
		t.Fatalf("clipped = %q", got)
	}
	trimmed := strings.TrimSuffix(got, "...")
	switch trimmed[strings.LastIndexByte(trimmed, ' ')+1:] {
	case "alpha", "beta", "gamma":
	default:
		t.Fatalf("clipped inside a word: %q", got)
	}
}

func TestAShortStepIsKept(t *testing.T) {
	if got := cleanStepMessage("Saved the answer", MaxStepMessageRunes); got != "Saved the answer" {
		t.Fatalf("got %q", got)
	}
}

func TestAClippedToolNameGetsNoEllipsis(t *testing.T) {
	if got := cleanStepText("abcdefgh", 4); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestATinyLimitIsCutWithoutAnEllipsis(t *testing.T) {
	if got := cleanStepMessage("abcdefgh", 2); got != "ab" {
		t.Fatalf("got %q", got)
	}
}
