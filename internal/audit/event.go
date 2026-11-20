package audit

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeDenied  Outcome = "denied"
	OutcomeFailed  Outcome = "failed"
)

type ActorKind string

const (
	ActorUser           ActorKind = "user"
	ActorServiceAccount ActorKind = "service_account"
	ActorAnonymous      ActorKind = "anonymous"
	ActorSystem         ActorKind = "system"
)

type Initiator string

const (
	InitiatorPerson Initiator = "person"
	InitiatorAgent  Initiator = "agent"
	InitiatorSystem Initiator = "system"
)

type AuthMethod string

const (
	AuthSession    AuthMethod = "session"
	AuthAPIKey     AuthMethod = "api_key"
	AuthOAuth      AuthMethod = "oauth"
	AuthPublicLink AuthMethod = "public_link"
	AuthNone       AuthMethod = "none"
)

const (
	OriginWeb = "web"
	OriginAPI = "api"
	OriginJob = "job"
)

type Target struct {
	Kind          string
	ID            string
	Name          string
	FolderID      uuid.NullUUID
	DashboardID   uuid.NullUUID
	ObservationID uuid.NullUUID
}

type Actor struct {
	Kind ActorKind
	ID   uuid.NullUUID
	Name string
	Role string
}

type Agent struct {
	ID   uuid.NullUUID
	Name string
}

type Auth struct {
	Method       AuthMethod
	CredentialID string
}

type Origin struct {
	Kind   string
	Client string
	Tool   string
}

type Scope struct {
	RequestID uuid.UUID
	TraceID   string
	Actor     Actor
	Initiator Initiator
	Agent     Agent
	RunID     uuid.NullUUID
	Auth      Auth
	Origin    Origin
	IP        netip.Addr
	UserAgent string
	Method    string
	Route     string
}

type By struct {
	Actor  Actor
	Origin Origin
	RunID  uuid.NullUUID
}

type Event struct {
	Action     Action
	Outcome    Outcome
	Reason     string
	Status     int
	Target     Target
	Thread     uuid.NullUUID
	Chat       uuid.NullUUID
	Datasource string
	Duration   time.Duration
	Data       map[string]any
	By         *By
}
