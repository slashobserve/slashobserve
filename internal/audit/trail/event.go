package trail

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Event struct {
	ID            string          `json:"id"`
	Seq           int64           `json:"seq"`
	OccurredAt    string          `json:"occurred_at"`
	Category      string          `json:"category"`
	Action        string          `json:"action"`
	Outcome       string          `json:"outcome"`
	Reason        *string         `json:"reason"`
	Status        *int32          `json:"status"`
	DurationMs    *int32          `json:"duration_ms"`
	ActorKind     string          `json:"actor_kind"`
	ActorID       *string         `json:"actor_id"`
	ActorName     *string         `json:"actor_name"`
	ActorRole     *string         `json:"actor_role"`
	Initiator     string          `json:"initiator"`
	AgentID       *string         `json:"agent_id"`
	AgentName     *string         `json:"agent_name"`
	RunID         *string         `json:"run_id"`
	AuthMethod    *string         `json:"auth_method"`
	CredentialID  *string         `json:"credential_id"`
	Origin        string          `json:"origin"`
	OriginClient  *string         `json:"origin_client"`
	Tool          *string         `json:"tool"`
	ClientIP      *string         `json:"client_ip"`
	UserAgent     *string         `json:"user_agent"`
	RequestID     *string         `json:"request_id"`
	TraceID       *string         `json:"trace_id"`
	HTTPMethod    *string         `json:"http_method"`
	Route         *string         `json:"route"`
	TargetKind    *string         `json:"target_kind"`
	TargetID      *string         `json:"target_id"`
	TargetName    *string         `json:"target_name"`
	FolderID      *string         `json:"folder_id"`
	DashboardID   *string         `json:"dashboard_id"`
	ObservationID *string         `json:"observation_id"`
	ThreadID      *string         `json:"thread_id"`
	ChatID        *string         `json:"chat_id"`
	DatasourceUID *string         `json:"datasource_uid"`
	Data          json.RawMessage `json:"data"`
	ServerID      string          `json:"server_id"`
	ServerVersion string          `json:"server_version"`

	at time.Time
}

type scannedEvent struct {
	id, actorID, agentID, runID, requestID, folderID, dashboardID, observationID, threadID, chatID pgtype.UUID
	seq                                                                                            int64
	occurredAt                                                                                     time.Time
	category, action, outcome, actorKind, initiator, origin, serverID, serverVersion               string
	reason, actorName, actorRole, agentName, authMethod, credentialID, originClient, tool          pgtype.Text
	userAgent, traceID, httpMethod, route, targetKind, targetID, targetName, datasourceUID         pgtype.Text
	status, durationMs                                                                             pgtype.Int4
	clientIP                                                                                       *netip.Addr
	data                                                                                           []byte
}

func (s *scannedEvent) targets() []any {
	return []any{
		&s.id, &s.seq, &s.occurredAt, &s.category, &s.action, &s.outcome, &s.reason, &s.status, &s.durationMs,
		&s.actorKind, &s.actorID, &s.actorName, &s.actorRole, &s.initiator, &s.agentID, &s.agentName, &s.runID,
		&s.authMethod, &s.credentialID, &s.origin, &s.originClient, &s.tool, &s.clientIP, &s.userAgent,
		&s.requestID, &s.traceID, &s.httpMethod, &s.route, &s.targetKind, &s.targetID, &s.targetName,
		&s.folderID, &s.dashboardID, &s.observationID, &s.threadID, &s.chatID, &s.datasourceUID, &s.data,
		&s.serverID, &s.serverVersion,
	}
}

func scanEvent(rows pgx.Rows, extra ...any) (Event, error) {
	var s scannedEvent
	if err := rows.Scan(append(s.targets(), extra...)...); err != nil {
		return Event{}, err
	}
	e := Event{
		ID: uuid.UUID(s.id.Bytes).String(), Seq: s.seq, OccurredAt: s.occurredAt.UTC().Format(time.RFC3339Nano), at: s.occurredAt.UTC(),
		Category: s.category, Action: s.action, Outcome: s.outcome, Reason: optText(s.reason),
		Status: optInt(s.status), DurationMs: optInt(s.durationMs),
		ActorKind: s.actorKind, ActorID: optUUID(s.actorID), ActorName: optText(s.actorName), ActorRole: optText(s.actorRole),
		Initiator: s.initiator, AgentID: optUUID(s.agentID), AgentName: optText(s.agentName), RunID: optUUID(s.runID),
		AuthMethod: optText(s.authMethod), CredentialID: optText(s.credentialID),
		Origin: s.origin, OriginClient: optText(s.originClient), Tool: optText(s.tool), UserAgent: optText(s.userAgent),
		RequestID: optUUID(s.requestID), TraceID: optText(s.traceID), HTTPMethod: optText(s.httpMethod), Route: optText(s.route),
		TargetKind: optText(s.targetKind), TargetID: optText(s.targetID), TargetName: optText(s.targetName),
		FolderID: optUUID(s.folderID), DashboardID: optUUID(s.dashboardID), ObservationID: optUUID(s.observationID),
		ThreadID: optUUID(s.threadID), ChatID: optUUID(s.chatID), DatasourceUID: optText(s.datasourceUID),
		Data: json.RawMessage(s.data), ServerID: s.serverID, ServerVersion: s.serverVersion,
	}
	if len(e.Data) == 0 {
		e.Data = json.RawMessage(`{}`)
	}
	if s.clientIP != nil {
		ip := s.clientIP.String()
		e.ClientIP = &ip
	}
	return e, nil
}

func optText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func optInt(n pgtype.Int4) *int32 {
	if !n.Valid {
		return nil
	}
	return &n.Int32
}

func optUUID(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := uuid.UUID(u.Bytes).String()
	return &s
}

var csvHeader = []string{
	"id", "seq", "occurred_at", "category", "action", "outcome", "reason", "status", "duration_ms",
	"actor_kind", "actor_id", "actor_name", "actor_role", "initiator", "agent_id", "agent_name", "run_id",
	"auth_method", "credential_id", "origin", "origin_client", "tool", "client_ip", "user_agent",
	"request_id", "trace_id", "http_method", "route", "target_kind", "target_id", "target_name",
	"folder_id", "dashboard_id", "observation_id", "thread_id", "chat_id", "datasource_uid", "data",
	"server_id", "server_version",
}

func (e Event) csvRecord() []string {
	return []string{
		e.ID, strconv.FormatInt(e.Seq, 10), e.OccurredAt, e.Category, e.Action, e.Outcome,
		str(e.Reason), num(e.Status), num(e.DurationMs),
		e.ActorKind, str(e.ActorID), str(e.ActorName), str(e.ActorRole), e.Initiator, str(e.AgentID), str(e.AgentName), str(e.RunID),
		str(e.AuthMethod), str(e.CredentialID), e.Origin, str(e.OriginClient), str(e.Tool), str(e.ClientIP), str(e.UserAgent),
		str(e.RequestID), str(e.TraceID), str(e.HTTPMethod), str(e.Route), str(e.TargetKind), str(e.TargetID), str(e.TargetName),
		str(e.FolderID), str(e.DashboardID), str(e.ObservationID), str(e.ThreadID), str(e.ChatID), str(e.DatasourceUID), string(e.Data),
		e.ServerID, e.ServerVersion,
	}
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func num(n *int32) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(int(*n))
}
