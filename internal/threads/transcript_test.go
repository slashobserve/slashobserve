package threads

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func transcriptMessage(id, run byte, author, body string) dbsqlc.ThreadMessage {
	m := dbsqlc.ThreadMessage{ID: pgtype.UUID{Bytes: [16]byte{id}, Valid: true}, AuthorKind: "user", AuthorName: author, Body: body}
	if run != 0 {
		m.RunID = pgtype.UUID{Bytes: [16]byte{run}, Valid: true}
		m.RunAssignedKind = pgtype.Text{String: "builtin", Valid: true}
	}
	return m
}

func TestTheTranscriptCarriesTheFindingOfAnAnswerInsteadOfItsStubBody(t *testing.T) {
	newestFirst := []dbsqlc.ThreadMessage{
		transcriptMessage(3, 0, "Ada", "which of those had errors?"),
		transcriptMessage(2, 9, "Ada", replyAnswered),
		transcriptMessage(1, 0, "Ada", "@observe what is slow?"),
	}
	answers := map[pgtype.UUID]Answer{
		newestFirst[1].RunID: {Title: "Checkout is slow", Markdown: "The 5xx rate rose after 14:00."},
	}

	got := transcriptOf(newestFirst, answers)

	if strings.Contains(got, replyAnswered) {
		t.Fatalf("the transcript still carries the stub: %q", got)
	}
	for _, want := range []string{"Checkout is slow", "The 5xx rate rose after 14:00.", "which of those had errors?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript lacks %q: %q", want, got)
		}
	}
}

func TestTheTranscriptKeepsTheBodyOfAReplyWithoutAnAnswer(t *testing.T) {
	newestFirst := []dbsqlc.ThreadMessage{transcriptMessage(2, 9, "Ada", "Saved version 3 of this dashboard.")}

	got := transcriptOf(newestFirst, map[pgtype.UUID]Answer{})

	if !strings.HasSuffix(got, "Saved version 3 of this dashboard.") {
		t.Fatalf("transcript = %q", got)
	}
}
