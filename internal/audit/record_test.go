package audit

import (
	"encoding/json"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingMetrics struct {
	mu       sync.Mutex
	dropped  map[string]int
	buffered int
	ahead    int
	batches  chan time.Duration
}

func newCountingMetrics() *countingMetrics {
	return &countingMetrics{dropped: map[string]int{}, batches: make(chan time.Duration, 64)}
}

func (m *countingMetrics) EventsDropped(reason string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropped[reason] += n
}

func (m *countingMetrics) BatchWritten(took time.Duration) { m.batches <- took }

func (m *countingMetrics) BufferedEvents(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buffered = n
}

func (m *countingMetrics) PartitionsAhead(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ahead = n
}

func (m *countingMetrics) droppedFor(reason string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped[reason]
}

var (
	ada     = Actor{Kind: ActorUser, ID: uuid.NullUUID{UUID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), Valid: true}, Name: "Ada", Role: "editor"}
	someone = Actor{Kind: ActorUser, ID: uuid.NullUUID{UUID: uuid.MustParse("22222222-2222-4222-8222-222222222222"), Valid: true}, Name: "Bo"}
	runID   = uuid.NullUUID{UUID: uuid.MustParse("33333333-3333-4333-8333-333333333333"), Valid: true}
)

func TestUndeclaredDataKeysAreDroppedAndCounted(t *testing.T) {
	m := newCountingMetrics()
	r := New(Config{Metrics: m})
	row, err := r.row(t.Context(), Event{Action: "dashboard.update", Data: map[string]any{"version": 3, "title": "Checkout", "spec": "{}", "prompt": "secret words"}})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(row.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 || data["version"] != float64(3) || data["title"] != "Checkout" {
		t.Fatalf("data = %v, want only version and title", data)
	}
	if got := m.droppedFor(DroppedUndeclaredKey); got != 2 {
		t.Fatalf("dropped undeclared keys = %d, want 2", got)
	}
}

func TestAnUnknownActionIsRefused(t *testing.T) {
	_, err := New(Config{}).row(t.Context(), Event{Action: "dashboard.explode"})
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
}

func TestRecordNowWithoutADatabaseSaysSo(t *testing.T) {
	if err := New(Config{}).RecordNow(t.Context(), Event{Action: ActionPrune}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func TestARowCarriesTheRequestScope(t *testing.T) {
	scope := Scope{
		RequestID: uuid.Must(uuid.NewV7()),
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		Actor:     ada,
		Initiator: InitiatorPerson,
		Auth:      Auth{Method: AuthSession, CredentialID: "44444444-4444-4444-8444-444444444444"},
		Origin:    Origin{Kind: OriginWeb},
		IP:        netip.MustParseAddr("192.0.2.7"),
		UserAgent: "test-agent",
		Method:    "POST",
		Route:     "/api/v1/dashboards/{id}/versions",
	}
	ctx := Open(t.Context(), scope)
	row, err := New(Config{Server: "s1", Version: "v9"}).row(ctx, Event{Action: "dashboard.update", Outcome: OutcomeDenied, Reason: "forbidden_role", Status: 403, Duration: 1500 * time.Millisecond, Target: Target{Kind: "dashboard", ID: "d1"}})
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case row.RequestID.Bytes != scope.RequestID:
		t.Errorf("request id = %v", row.RequestID)
	case row.ClientIp == nil || row.ClientIp.String() != "192.0.2.7":
		t.Errorf("client ip = %v", row.ClientIp)
	case row.ActorKind != "user" || row.ActorID.Bytes != ada.ID.UUID || row.ActorRole.String != "editor":
		t.Errorf("actor = %s %v %v", row.ActorKind, row.ActorID, row.ActorRole)
	case row.AuthMethod.String != "session" || row.CredentialID.String != scope.Auth.CredentialID:
		t.Errorf("auth = %v %v", row.AuthMethod, row.CredentialID)
	case row.Outcome != "denied" || row.Reason.String != "forbidden_role" || row.Status.Int32 != 403 || row.DurationMs.Int32 != 1500:
		t.Errorf("outcome = %s %v %v %v", row.Outcome, row.Reason, row.Status, row.DurationMs)
	case row.Route.String != scope.Route || row.HttpMethod.String != "POST" || row.TraceID.String != scope.TraceID || row.UserAgent.String != "test-agent":
		t.Errorf("request = %v %v %v %v", row.Route, row.HttpMethod, row.TraceID, row.UserAgent)
	case row.Category != "content" || row.TargetKind.String != "dashboard" || row.TargetID.String != "d1":
		t.Errorf("event = %s %v %v", row.Category, row.TargetKind, row.TargetID)
	case row.ServerID != "s1" || row.ServerVersion != "v9":
		t.Errorf("server = %s %s", row.ServerID, row.ServerVersion)
	case row.OccurredAt.Time.Location() != time.UTC || row.OccurredAt.Time.Nanosecond()%1000 != 0:
		t.Errorf("occurred at %v is not UTC microseconds", row.OccurredAt.Time)
	}
	if id, err := uuid.FromBytes(row.ID.Bytes[:]); err != nil || id.Version() != 7 {
		t.Errorf("event id %v is not a UUID v7", id)
	}
}

func TestAttribution(t *testing.T) {
	inRequest := Scope{Actor: ada, Initiator: InitiatorPerson, Auth: Auth{Method: AuthSession, CredentialID: "c"}, Origin: Origin{Kind: OriginWeb}}
	tests := []struct {
		name      string
		scope     Scope
		scoped    bool
		by        *By
		actor     Actor
		initiator Initiator
		origin    string
		auth      AuthMethod
	}{
		{name: "no scope is the system", actor: Actor{Kind: ActorSystem}, initiator: InitiatorSystem, origin: OriginJob},
		{name: "the request's principal", scope: inRequest, scoped: true, actor: ada, initiator: InitiatorPerson, origin: OriginWeb, auth: AuthSession},
		{name: "the same principal keeps its role and credential", scope: inRequest, scoped: true, by: &By{Actor: Actor{Kind: ActorUser, ID: ada.ID, Name: "Ada"}}, actor: ada, initiator: InitiatorPerson, origin: OriginWeb, auth: AuthSession},
		{name: "another actor drops the credential", scope: inRequest, scoped: true, by: &By{Actor: someone}, actor: someone, initiator: InitiatorPerson, origin: OriginWeb},
		{name: "a run is the agent", scope: inRequest, scoped: true, by: &By{Actor: someone, Origin: Origin{Kind: "agent"}, RunID: runID}, actor: someone, initiator: InitiatorAgent, origin: "agent"},
		{name: "a job acting for a person", by: &By{Actor: someone}, actor: someone, initiator: InitiatorPerson, origin: OriginJob},
		{name: "a job acting as the system", by: &By{Actor: Actor{Kind: ActorSystem}}, actor: Actor{Kind: ActorSystem}, initiator: InitiatorSystem, origin: OriginJob},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attribution(tt.scope, tt.scoped, tt.by)
			if got.Actor != tt.actor || got.Initiator != tt.initiator || got.Origin.Kind != tt.origin || got.Auth.Method != tt.auth {
				t.Fatalf("attribution = %+v %s %s %s, want %+v %s %s %s", got.Actor, got.Initiator, got.Origin.Kind, got.Auth.Method, tt.actor, tt.initiator, tt.origin, tt.auth)
			}
		})
	}
}

func TestScopeUpdatesAreSeenByLaterReads(t *testing.T) {
	ctx := Open(t.Context(), Scope{Actor: Actor{Kind: ActorAnonymous}})
	Update(ctx, func(s *Scope) { s.Actor = ada })
	if got, ok := ScopeOf(ctx); !ok || got.Actor != ada {
		t.Fatalf("scope = %+v %v", got, ok)
	}
	if _, ok := ScopeOf(t.Context()); ok {
		t.Fatal("a context without a scope reports one")
	}
	Update(t.Context(), func(*Scope) { t.Fatal("update ran without a scope") })
}
