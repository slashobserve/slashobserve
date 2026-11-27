package runsession

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const sweptBlock = "<triage>\nCandidates: none.\n</triage>"

type countingSweeper struct {
	runs atomic.Int64
	last atomic.Value
}

func (c *countingSweeper) Sweep(ctx context.Context, run tools.Run) triage.Outcome {
	c.runs.Add(1)
	access, _ := datasource.AccessFrom(datasource.WithAccess(ctx, run.Access))
	c.last.Store(access)
	return triage.Outcome{Block: sweptBlock, Calls: 7, Duration: 1500 * time.Millisecond}
}

func TestTheFirstBriefOfAnInvestigationSweepsOnce(t *testing.T) {
	r := newRig(t)
	sweeper := &countingSweeper{}
	r.svc.SweepWith(sweeper)
	req := r.investigate()
	req.Investigation = true
	for _, reason := range []string{"", "", "renewed", "restart"} {
		got, err := r.svc.Brief(context.Background(), req, reason)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Message, sweptBlock) {
			t.Fatalf("brief %q lacks the triage block:\n%s", reason, got.Message)
		}
	}
	if n := sweeper.runs.Load(); n != 1 {
		t.Fatalf("swept %d times, want once", n)
	}
	var swept []string
	for _, m := range r.steps.messages() {
		if strings.Contains(m, "Swept the datasources") {
			swept = append(swept, m)
		}
	}
	if len(swept) != 1 || !strings.Contains(swept[0], "7 reads in 1.5s") {
		t.Fatalf("sweep steps = %v", swept)
	}
	if access := sweeper.last.Load().(datasource.Access); access.By.ID != req.RequestedBy.ID {
		t.Fatalf("the sweep read as %+v, want the requester", access.By)
	}
}

func TestOnlyInvestigationsSweep(t *testing.T) {
	r := newRig(t)
	sweeper := &countingSweeper{}
	r.svc.SweepWith(sweeper)
	ask := r.investigate()
	ask.Investigation = false
	revise := r.revise()
	for _, req := range []revisions.Request{revise, ask} {
		got, err := r.svc.Brief(context.Background(), req, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got.Message, "<triage>") {
			t.Fatalf("a run that is not an investigation got the block:\n%s", got.Message)
		}
	}
	if n := sweeper.runs.Load(); n != 0 {
		t.Fatalf("swept %d times", n)
	}
}

func TestANewAttemptKeepsTheSweepOfTheLastOne(t *testing.T) {
	r := newRig(t)
	sweeper := &countingSweeper{}
	r.svc.SweepWith(sweeper)
	req := r.investigate()
	req.Investigation = true
	id := mustUUID(t, req.ID)
	r.d.Exec(t, `INSERT INTO run_sessions (agent_request_id, attempt, state) VALUES ($1, $2, $3)`, id, req.Attempt, `{"guard":{},"triage":{"block":"<triage>\nfrom the last attempt\n</triage>","calls":3,"duration_ms":900}}`)
	r.d.Exec(t, `UPDATE agent_requests SET attempts = attempts + 1 WHERE id = $1`, id)
	req.Attempt++
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Message, "from the last attempt") {
		t.Fatalf("brief lacks the earlier sweep:\n%s", got.Message)
	}
	if n := sweeper.runs.Load(); n != 0 {
		t.Fatalf("swept %d times, want the stored block reused", n)
	}
}

type leadSweeper struct{}

func (leadSweeper) Sweep(context.Context, tools.Run) triage.Outcome {
	return triage.Outcome{Block: "<triage>\nc1 payment\nc2 ad\n</triage>", Leads: twoLeads, Calls: 9, Duration: time.Second}
}

func TestTheSweepSeedsTheLedgerOfTheFirstBriefAndANewAttemptSeedsItAgain(t *testing.T) {
	r := newRig(t, withInvestigationLoop())
	r.svc.SweepWith(leadSweeper{})
	req := r.investigate()
	req.Investigation = true
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- c1 [open] " + twoLeads[0].Text, "- c2 [open] " + twoLeads[1].Text} {
		if !strings.Contains(got.Message, want) {
			t.Fatalf("the first brief lacks %q:\n%s", want, got.Message)
		}
	}
	id := mustUUID(t, req.ID)
	r.d.Exec(t, `UPDATE agent_requests SET attempts = attempts + 1 WHERE id = $1`, id)
	req.Attempt++
	got, err = r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Message, "- c2 [open] "+twoLeads[1].Text) {
		t.Fatalf("the next attempt's brief lacks the seeded ledger:\n%s", got.Message)
	}
}
