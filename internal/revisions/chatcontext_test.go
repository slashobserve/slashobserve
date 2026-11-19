package revisions

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChatTranscriptKeepsTurnsOldestFirst(t *testing.T) {
	got := chatTranscript([]chatTurn{
		{prompt: "which logs exist", answer: "Loki and Elasticsearch"},
		{prompt: "show errors", answer: "Saved version 2 of the dashboard: error rate"},
	})
	want := "You: which logs exist\nObserve Agent: Loki and Elasticsearch\nYou: show errors\nObserve Agent: Saved version 2 of the dashboard: error rate"
	if got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

func TestChatTranscriptSaysWhenATurnSavedNothing(t *testing.T) {
	got := chatTranscript([]chatTurn{{prompt: "hello"}})
	if !strings.Contains(got, "Observe Agent: No answer or dashboard was saved.") {
		t.Fatalf("transcript = %q", got)
	}
}

func TestChatTranscriptClipsLongPromptsAndAnswers(t *testing.T) {
	got := chatTranscript([]chatTurn{{prompt: strings.Repeat("p", 5000), answer: strings.Repeat("a", 5000)}})
	if want := maxContextPromptRunes + maxContextAnswerRunes + len("You: \nObserve Agent: ") + 2*len("..."); utf8.RuneCountInString(got) != want {
		t.Fatalf("length = %d, want %d", utf8.RuneCountInString(got), want)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("a clipped answer should end with an ellipsis")
	}
}

func TestChatTranscriptDropsTheOldestTurnsFirstWhenOverBudget(t *testing.T) {
	var turns []chatTurn
	for _, marker := range []string{"first", "second", "third", "fourth", "fifth", "sixth"} {
		turns = append(turns, chatTurn{prompt: marker + strings.Repeat("p", 595), answer: strings.Repeat("a", 1200)})
	}
	got := chatTranscript(turns)
	if utf8.RuneCountInString(got) > maxThreadContextRunes {
		t.Fatalf("transcript is %d runes, over the %d budget", utf8.RuneCountInString(got), maxThreadContextRunes)
	}
	if strings.Contains(got, "first") || !strings.Contains(got, "sixth") {
		t.Fatalf("budget should drop the oldest turn and keep the newest")
	}
	if strings.Index(got, "fifth") > strings.Index(got, "sixth") {
		t.Fatalf("turns are not oldest first")
	}
}

func TestChatTranscriptOfNoTurnsIsEmpty(t *testing.T) {
	if got := chatTranscript(nil); got != "" {
		t.Fatalf("transcript = %q, want empty", got)
	}
}
