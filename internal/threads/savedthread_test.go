package threads

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/cli"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type savedThreadAnswer struct {
	asker  auth.Principal
	about  pgtype.UUID
	thread string
	run    pgtype.UUID
	saved  pgtype.UUID
}

func (r *rig) savedThreadAnswer(t *testing.T) savedThreadAnswer {
	t.Helper()
	asker := testUser
	asker.Groups = []string{payrollGroup}
	restricted := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	r.db.Exec(t, `UPDATE folder_permissions SET level = 'edit' WHERE folder_id = $1`, restricted)
	about := r.filedObservationIn(t, asker, restricted)
	opened, _, started, refusal := r.openWithRun(t, asker, OpenInput{Target: target(KindObservation, about), Body: "@observe look at this"})
	if started == nil || refusal != nil {
		t.Fatalf("started = %+v, refusal = %+v", started, refusal)
	}
	run := mustParse(t, started.ID)
	r.answer(t, asker, findings.SaveInput{AgentRequestID: run})
	r.finishWithAnswer(t, run)
	saved, _, err := r.findings.SaveAsObservation(reading(t.Context()), asker, run)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, mustParse(t, saved.ObservationID), foldertest.Open(t, r.db, "Shared"))
	return savedThreadAnswer{asker: asker, about: about, thread: opened.ID, run: run, saved: mustParse(t, saved.ObservationID)}
}

func (r *rig) finishWithAnswer(t *testing.T, run pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now() WHERE id = $1`, run)
	if err := r.runEnded(t, r.runRow(t, run)); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) runRow(t *testing.T, run pgtype.UUID) dbsqlc.AgentRequest {
	t.Helper()
	row, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), run)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (r *rig) messagesIn(t *testing.T, thread string) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT count(*) FROM thread_messages WHERE thread_id = $1`, thread).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) observeEngine(who auth.Principal) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	revisions.NewObserveHandler(r.revisions, revisions.ObserveOptions{}).Register(auth.NewRoutes(engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	})))
	return engine
}

func retriedOverHTTP(t *testing.T, engine http.Handler, run pgtype.UUID) pgtype.UUID {
	t.Helper()
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/observe/"+run.String()+"/retry", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry answered %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Request struct {
			ID string `json:"id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return mustParse(t, body.Request.ID)
}

func retriedOverCLI(t *testing.T, engine http.Handler, run pgtype.UUID) pgtype.UUID {
	t.Helper()
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := cli.Main([]string{"--server", server.URL, "--token", "k", "retry", run.String(), "--no-wait"}, cli.IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 0 {
		t.Fatalf("observe retry exited %d: %s", code, stderr.String())
	}
	return mustParse(t, strings.TrimSpace(stdout.String()))
}

func TestARetryOfASavedThreadAnswerBySomeoneOutsideTheThreadIsTheirOwnAskAboutTheSavedObservation(t *testing.T) {
	r := newShareRig(t)
	s := r.savedThreadAnswer(t)
	messages := r.messagesIn(t, s.thread)
	engine := r.observeEngine(commenter)

	for name, retry := range map[string]func(*testing.T, http.Handler, pgtype.UUID) pgtype.UUID{"http": retriedOverHTTP, "cli": retriedOverCLI} {
		t.Run(name, func(t *testing.T) {
			next := r.runRow(t, retry(t, engine, s.run))
			if next.ThreadMessageID.Valid || next.ThreadContext.String != "" {
				t.Fatalf("the retry carries thread message %v and context %q of a thread its requester cannot read", next.ThreadMessageID, next.ThreadContext.String)
			}
			if next.RequestedByID.String() != commenter.ID || !revisions.PrivateAsk(next) || next.SubjectObservationID != s.saved {
				t.Fatalf("retry %+v, want a private ask of %s about %v", next, commenter.Name, s.saved)
			}
			r.finishWithAnswer(t, next.ID)
			if got := r.messagesIn(t, s.thread); got != messages {
				t.Fatalf("the thread has %d messages after the retry ended, want %d", got, messages)
			}
		})
	}
}

func TestARetryOfASavedThreadAnswerBySomeoneInTheThreadAnswersInIt(t *testing.T) {
	r := newShareRig(t)
	s := r.savedThreadAnswer(t)

	next := r.runRow(t, retriedOverHTTP(t, r.observeEngine(s.asker), s.run))

	if !next.ThreadMessageID.Valid || next.ChatID.Valid || next.SubjectObservationID != s.about || next.ObservationID.Valid {
		t.Fatalf("retry %+v, want a thread ask about %v", next, s.about)
	}
}

func TestASavedThreadAnswerIsStillReadFromTheThreadByThoseWhoCannotSeeTheSavedObservation(t *testing.T) {
	r := newShareRig(t)
	about := r.filedObservationIn(t, testUser, foldertest.Open(t, r.db, "Shared"))
	_, _, started, refusal := r.openWithRun(t, testUser, OpenInput{Target: target(KindObservation, about), Body: "@observe look at this"})
	if started == nil || refusal != nil {
		t.Fatalf("started = %+v, refusal = %+v", started, refusal)
	}
	run := mustParse(t, started.ID)
	answer := r.answer(t, testUser, findings.SaveInput{AgentRequestID: run})
	r.finishWithAnswer(t, run)
	saved := r.savedAsObservation(t, run, about)
	r.db.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1`, saved, foldertest.AdminsOnly(t, r.db, "Admins"))

	for _, reader := range []auth.Principal{commenter, viewer} {
		if code := r.observeStatus(t, reader, run); code != http.StatusOK {
			t.Errorf("%s reads the run with %d", reader.Name, code)
		}
		f, err := r.findingOfRun(t, reader, run)
		if err != nil || f.ID != answer.ID {
			t.Errorf("%s reads the run's answer %+v, %v", reader.Name, f, err)
		}
		got, err := r.findings.Get(reading(t.Context()), reader, mustParse(t, answer.ID))
		if err != nil || got.Folder != nil || got.ObservationID != "" || got.FolderID != "" {
			t.Errorf("%s reads the answer %+v, %v, want it without the saved observation or its folder", reader.Name, got, err)
		}
	}
}

func TestAReplyToAThreadAnswerSavedElsewhereIsOneSentence(t *testing.T) {
	r := newShareRig(t)
	s := r.savedThreadAnswer(t)

	body, err := succeededBody(reading(t.Context()), r.db.Queries(), r.runRow(t, s.run))

	if err != nil || body != "Answered." {
		t.Fatalf("a reply composed after the answer was saved in another observation reads %q, %v", body, err)
	}
}
