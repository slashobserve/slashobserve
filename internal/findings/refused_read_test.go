package findings_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func TestARefusedReadOfAnAnswerIsRecordedEveryTime(t *testing.T) {
	r := newRig(t)
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: r.request(t, editor, r.open), DashboardID: r.open})
	recorder := audit.New(audit.Config{DB: r.db.DB, Logger: slog.New(slog.DiscardHandler)})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()

	for range 2 {
		probe := audit.Open(auth.WithPrincipal(reading(t.Context()), otherEditor), audit.Scope{Actor: auth.AuditActor(otherEditor), Initiator: audit.InitiatorPerson, Origin: audit.Origin{Kind: "web"}})
		if _, err := r.service.Read(probe, otherEditor, mustUUID(t, answer.ID)); !errors.Is(err, findings.ErrNotFound) {
			t.Fatalf("another Editor reads the unsaved answer: %v, want ErrNotFound", err)
		}
	}
	stop()
	<-done

	var n int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM audit_events
		WHERE action = 'run.view' AND outcome = 'denied' AND target_kind = 'finding' AND target_id = $1 AND actor_id = $2::uuid`, answer.ID, otherEditor.ID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("refused reads recorded = %d, want 2", n)
	}
}
