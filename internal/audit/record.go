package audit

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

var (
	ErrUnknownAction = errors.New("audit action is not in the catalog")
	ErrNotInstalled  = errors.New("audit recorder is not installed")
)

const (
	DroppedUndeclaredKey = "undeclared_key"
	DroppedBufferFull    = "buffer_full"
	DroppedWriteFailed   = "write_failed"
	DroppedRecordFailed  = "record_failed"
)

type Metrics interface {
	EventsDropped(reason string, n int)
	BatchWritten(took time.Duration)
	BufferedEvents(n int)
	PartitionsAhead(n int)
}

type Config struct {
	DB        *db.DB
	Server    string
	Version   string
	Metrics   Metrics
	Retention time.Duration
	Logger    *slog.Logger
}

type Recorder struct {
	db        *db.DB
	server    string
	version   string
	metrics   Metrics
	retention time.Duration
	logger    *slog.Logger
	writer    *Writer
	inflight  atomic.Int64

	queryRepeats *Repeats
}

var installed atomic.Pointer[Recorder]

func Install(r *Recorder) {
	installed.Store(r)
}

func New(cfg Config) *Recorder {
	r := &Recorder{
		db:        cfg.DB,
		server:    cfg.Server,
		version:   cfg.Version,
		metrics:   cfg.Metrics,
		retention: cfg.Retention,
		logger:    cfg.Logger,

		queryRepeats: NewRepeats(),
	}
	if r.server == "" {
		r.server, _ = os.Hostname()
	}
	if r.version == "" {
		r.version = "dev"
	}
	if r.metrics == nil {
		r.metrics = nopMetrics{}
	}
	if r.logger == nil {
		r.logger = slog.New(slog.DiscardHandler)
	}
	r.writer = newWriter(r, writerLimits{})
	return r
}

var unconfigured = New(Config{})

func current() *Recorder {
	if r := installed.Load(); r != nil {
		return r
	}
	return unconfigured
}

func Record(ctx context.Context, q *dbsqlc.Queries, e Event) error {
	return current().Record(ctx, q, e)
}

func RecordNow(ctx context.Context, e Event) error {
	return current().RecordNow(ctx, e)
}

func Dropped(reason string) {
	current().metrics.EventsDropped(reason, 1)
}

func (r *Recorder) Record(ctx context.Context, q *dbsqlc.Queries, e Event) error {
	row, err := r.row(ctx, e)
	if err != nil {
		return err
	}
	if err := q.InsertAuditEvent(ctx, row); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	if st := stateOf(ctx); st != nil {
		st.wroteInTx(written{at: row.OccurredAt.Time, id: row.ID.Bytes, action: e.Action})
	}
	return nil
}

func (r *Recorder) RecordNow(ctx context.Context, e Event) error {
	if r.db == nil {
		return ErrNotInstalled
	}
	row, err := r.row(ctx, e)
	if err != nil {
		return err
	}
	if batched(e.Action, Outcome(row.Outcome)) {
		r.writer.enqueue(row)
	} else if err := r.insertNow(ctx, row); err != nil {
		return err
	}
	if st := stateOf(ctx); st != nil {
		st.markCommitted()
	}
	return nil
}

func (r *Recorder) insertNow(ctx context.Context, row dbsqlc.InsertAuditEventParams) error {
	if r.db == nil {
		return ErrNotInstalled
	}
	ictx, cancel := context.WithTimeout(ctx, r.writer.limits.timeout)
	defer cancel()
	err := r.db.Queries().InsertAuditEvent(ictx, row)
	if err == nil {
		return nil
	}
	if retryable(err) && r.writer.handOff(row) {
		r.logger.WarnContext(ctx, "audit event queued to be written again", logattrs.Failure("audit_write_failed", err)...)
		return nil
	}
	return fmt.Errorf("insert audit event: %w", err)
}

func (r *Recorder) row(ctx context.Context, e Event) (dbsqlc.InsertAuditEventParams, error) {
	spec, ok := Lookup(e.Action)
	if !ok {
		return dbsqlc.InsertAuditEventParams{}, fmt.Errorf("%w: %q", ErrUnknownAction, e.Action)
	}
	data, err := r.declaredData(spec, e.Outcome, e.Data)
	if err != nil {
		return dbsqlc.InsertAuditEventParams{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return dbsqlc.InsertAuditEventParams{}, fmt.Errorf("audit event id: %w", err)
	}
	scope, scoped := ScopeOf(ctx)
	who := attribution(scope, scoped, e.By)
	outcome := e.Outcome
	if outcome == "" {
		outcome = OutcomeSuccess
	}
	row := dbsqlc.InsertAuditEventParams{
		ID:            pgtype.UUID{Bytes: id, Valid: true},
		OccurredAt:    pgtype.Timestamptz{Time: time.Now().UTC().Truncate(time.Microsecond), Valid: true},
		Category:      string(spec.Category),
		Action:        string(e.Action),
		Outcome:       string(outcome),
		Reason:        text(e.Reason),
		ActorKind:     string(who.Actor.Kind),
		ActorID:       nullUUID(who.Actor.ID),
		ActorName:     text(who.Actor.Name),
		ActorRole:     text(who.Actor.Role),
		Initiator:     string(who.Initiator),
		AgentID:       nullUUID(who.Agent.ID),
		AgentName:     text(who.Agent.Name),
		RunID:         nullUUID(who.RunID),
		AuthMethod:    text(string(who.Auth.Method)),
		CredentialID:  text(who.Auth.CredentialID),
		Origin:        who.Origin.Kind,
		OriginClient:  text(who.Origin.Client),
		Tool:          text(who.Origin.Tool),
		UserAgent:     text(who.UserAgent),
		TraceID:       text(who.TraceID),
		HttpMethod:    text(who.Method),
		Route:         text(who.Route),
		TargetKind:    text(e.Target.Kind),
		TargetID:      text(e.Target.ID),
		TargetName:    text(e.Target.Name),
		FolderID:      nullUUID(e.Target.FolderID),
		DashboardID:   nullUUID(e.Target.DashboardID),
		ObservationID: nullUUID(e.Target.ObservationID),
		ThreadID:      nullUUID(e.Thread),
		ChatID:        nullUUID(e.Chat),
		DatasourceUid: text(e.Datasource),
		Data:          data,
		ServerID:      r.server,
		ServerVersion: r.version,
	}
	if e.Status != 0 {
		row.Status = int4(int64(e.Status))
	}
	if e.Activity > 0 {
		row.ActivitySeq = pgtype.Int8{Int64: e.Activity, Valid: true}
	}
	if e.Duration > 0 {
		row.DurationMs = int4(e.Duration.Milliseconds())
	}
	if scoped {
		row.RequestID = pgtype.UUID{Bytes: scope.RequestID, Valid: true}
		if who.IP.IsValid() {
			ip := who.IP
			row.ClientIp = &ip
		}
	}
	return row, nil
}

func (r *Recorder) declaredData(spec Spec, outcome Outcome, data map[string]any) ([]byte, error) {
	kept := make(map[string]any, len(data))
	dropped := 0
	for key, value := range data {
		if slices.Contains(spec.Data, key) || outcome == OutcomeDenied && slices.Contains(refusalKeys, key) {
			kept[key] = value
		} else {
			dropped++
		}
	}
	if dropped > 0 {
		r.metrics.EventsDropped(DroppedUndeclaredKey, dropped)
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("marshal audit data: %w", err)
	}
	return raw, nil
}

func attribution(scope Scope, scoped bool, by *By) Scope {
	if !scoped {
		scope = Scope{Actor: Actor{Kind: ActorSystem}, Initiator: InitiatorSystem, Origin: Origin{Kind: OriginJob}}
	}
	if by == nil {
		return scope
	}
	actor := by.Actor
	if actor.Kind == scope.Actor.Kind && actor.ID == scope.Actor.ID {
		actor.Role = cmp.Or(actor.Role, scope.Actor.Role)
	} else {
		scope.Auth = Auth{}
	}
	scope.Actor = actor
	if by.Agent != (Agent{}) {
		scope.Agent = by.Agent
	}
	if by.Origin.Kind != "" && scope.Origin.Kind != OriginAgent {
		scope.Origin = by.Origin
	}
	switch {
	case by.RunID.Valid:
		scope.RunID = by.RunID
		scope.Initiator = InitiatorAgent
	case actor.Kind == ActorSystem:
		scope.Initiator = InitiatorSystem
	case scope.Initiator == InitiatorSystem:
		scope.Initiator = InitiatorPerson
	}
	return scope
}

func int4(n int64) pgtype.Int4 {
	switch {
	case n > math.MaxInt32:
		n = math.MaxInt32
	case n < math.MinInt32:
		n = math.MinInt32
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func nullUUID(u uuid.NullUUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u.UUID, Valid: u.Valid}
}

type nopMetrics struct{}

func (nopMetrics) EventsDropped(string, int)  {}
func (nopMetrics) BatchWritten(time.Duration) {}
func (nopMetrics) BufferedEvents(int)         {}
func (nopMetrics) PartitionsAhead(int)        {}
