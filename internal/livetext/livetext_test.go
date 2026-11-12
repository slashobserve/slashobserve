package livetext

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func newStore() *Store {
	return New(nil, nil, slog.New(slog.DiscardHandler))
}

func send(t *testing.T, s *Store, run string, attempt int, text string) {
	t.Helper()
	payload, err := encode(message{Run: run, Attempt: attempt, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	s.apply(payload)
}

func mustText(t *testing.T, s *Store, run string, attempt int) string {
	t.Helper()
	got, ok := s.Text(run, attempt)
	if !ok {
		t.Fatalf("no text for run %s attempt %d", run, attempt)
	}
	return got
}

func TestAppendedTextAccumulatesPerRun(t *testing.T) {
	s := newStore()
	send(t, s, "a", 1, "Looking at ")
	send(t, s, "a", 1, "the error rate.")
	send(t, s, "b", 1, "other run")
	if got := mustText(t, s, "a", 1); got != "Looking at the error rate." {
		t.Fatalf("run a = %q", got)
	}
	if got := mustText(t, s, "b", 1); got != "other run" {
		t.Fatalf("run b = %q", got)
	}
	if _, ok := s.Text("c", 1); ok {
		t.Fatal("an unknown run has text")
	}
}

func TestOnlyTheLastTwoThousandRunesAreKeptOnARuneBoundary(t *testing.T) {
	s := newStore()
	send(t, s, "a", 1, strings.Repeat("é", MaxRunes))
	send(t, s, "a", 1, "xyz")
	got := mustText(t, s, "a", 1)
	if utf8.RuneCountInString(got) != MaxRunes || !utf8.ValidString(got) {
		t.Fatalf("kept %d runes, valid %t", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	if !strings.HasSuffix(got, "éxyz") || !strings.HasPrefix(got, "é") {
		t.Fatalf("tail = %q...%q", got[:4], got[len(got)-6:])
	}
	send(t, s, "b", 1, strings.Repeat("日", MaxRunes+5))
	if got := mustText(t, s, "b", 1); utf8.RuneCountInString(got) != MaxRunes {
		t.Fatalf("a single oversized payload kept %d runes", utf8.RuneCountInString(got))
	}
}

func TestANewerAttemptReplacesAndAnOlderOneIsIgnored(t *testing.T) {
	s := newStore()
	send(t, s, "a", 2, "second attempt")
	send(t, s, "a", 1, "late first attempt")
	if got := mustText(t, s, "a", 2); got != "second attempt" {
		t.Fatalf("text = %q", got)
	}
	send(t, s, "a", 3, "third")
	if got := mustText(t, s, "a", 3); got != "third" {
		t.Fatalf("text = %q", got)
	}
	if _, ok := s.Text("a", 2); ok {
		t.Fatal("the replaced attempt still has text")
	}
}

func TestControlCharactersAndInvalidUTF8AreDropped(t *testing.T) {
	if got, want := clean("a\x00b\x1b[31mc\r\nd\te\xffg"), "ab[31mc\nd\teg"; got != want {
		t.Fatalf("clean = %q, want %q", got, want)
	}
	if clean("\x00\x01\xff") != "" {
		t.Fatal("only control characters left text behind")
	}
}

func TestLongTextIsChunkedIntoPayloadsUnderTheNotifyLimit(t *testing.T) {
	var sb strings.Builder
	for sb.Len() < 10*1024 {
		sb.WriteString(`日本語 "quoted" back\slash <b>&</b> ` + "\n\t\u2028")
	}
	text := clean(sb.String())
	parts := chunks(text)
	if len(parts) < 3 {
		t.Fatalf("%d chunks for %d bytes", len(parts), len(text))
	}
	var rebuilt strings.Builder
	for _, part := range parts {
		if !utf8.ValidString(part) {
			t.Fatalf("a chunk splits a rune: %q", part)
		}
		payload, err := encode(message{Run: "0f8fad5b-d9cb-469f-a165-70867728950e", Attempt: 12, Text: part})
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) >= 8000 {
			t.Fatalf("payload is %d bytes", len(payload))
		}
		if strings.Contains(payload, "\\u003c") {
			t.Fatalf("payload escapes HTML: %s", payload)
		}
		var back message
		if err := json.Unmarshal([]byte(payload), &back); err != nil {
			t.Fatal(err)
		}
		rebuilt.WriteString(back.Text)
	}
	if rebuilt.String() != text {
		t.Fatal("the chunks do not reassemble to the cleaned text")
	}
}

func TestChunkingEmptyTextSendsNothing(t *testing.T) {
	if got := chunks(clean("\x00\x1b")); len(got) != 0 {
		t.Fatalf("chunks = %q", got)
	}
}

func TestTheOldestRunIsEvictedAtTheEntryLimit(t *testing.T) {
	s := newStore()
	clock := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	for i := 0; i < maxEntries; i++ {
		clock = clock.Add(time.Millisecond)
		send(t, s, fmt.Sprintf("run-%d", i), 1, "text")
	}
	clock = clock.Add(time.Millisecond)
	send(t, s, "run-new", 1, "text")
	if _, ok := s.Text("run-0", 1); ok {
		t.Fatal("the oldest run was kept")
	}
	if _, ok := s.Text("run-1", 1); !ok {
		t.Fatal("the second oldest run was evicted")
	}
	if _, ok := s.Text("run-new", 1); !ok {
		t.Fatal("the new run is missing")
	}
	if len(s.runs) != maxEntries {
		t.Fatalf("%d entries kept, want %d", len(s.runs), maxEntries)
	}
}

func TestEntriesUntouchedForTenMinutesAreDroppedOnTheNextWrite(t *testing.T) {
	s := newStore()
	clock := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	send(t, s, "stale", 1, "text")
	clock = clock.Add(5 * time.Minute)
	send(t, s, "fresh", 1, "text")
	clock = clock.Add(5*time.Minute + time.Second)
	send(t, s, "other", 1, "text")
	if _, ok := s.Text("stale", 1); ok {
		t.Fatal("an entry untouched for over ten minutes was kept")
	}
	if _, ok := s.Text("fresh", 1); !ok {
		t.Fatal("an entry touched five minutes ago was dropped")
	}
}

func TestMalformedPayloadsAreDropped(t *testing.T) {
	s := newStore()
	for _, payload := range []string{"", "not json", `{"run":"","attempt":1,"text":"x"}`, `{"run":"a","attempt":"one"}`} {
		s.apply(payload)
	}
	if len(s.runs) != 0 {
		t.Fatalf("entries = %d, want 0", len(s.runs))
	}
}
