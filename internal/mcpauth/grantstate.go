package mcpauth

import (
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

const (
	GrantStateRevoked = "revoked"
	GrantStateActive  = "active"
	GrantStateExpired = "expired"
	GrantStateAgedOut = "aged_out"
)

var GrantStates = lifecycle.Derived{Name: "connected agent", States: []lifecycle.DerivedState{
	{Name: GrantStateRevoked, When: "It was ended before its time, and `revoked_reason` says how: by its person, an admin or the client, by a newer connection of the same client, or because a code or refresh token was used twice."},
	{Name: GrantStateActive, When: "It may call /mcp as its person until expires_at."},
	{Name: GrantStateExpired, When: "Its refresh token went unused for 30 days; the person approves the client again."},
	{Name: GrantStateAgedOut, When: "It reached the 90 day limit of a connection; the person approves the client again."},
}}

type grantFacts struct {
	revoked bool
	created time.Time
	expires time.Time
	now     time.Time
}

func grantStateOf(f grantFacts) string {
	switch {
	case f.revoked:
		return GrantStateRevoked
	case f.expires.After(f.now):
		return GrantStateActive
	case !f.expires.Before(f.created.Add(grantMaxAge)):
		return GrantStateAgedOut
	}
	return GrantStateExpired
}

type endedGrant struct {
	state         string
	revokedReason string
}

func (e *endedGrant) Error() string { return errInvalidGrant.code }

var revokedDescriptions = map[string]string{
	reasonPerson:    "the person disconnected this agent; connect again to ask for their approval",
	reasonAdmin:     "an admin disconnected this agent; connect again to ask for approval",
	"client":        "the client revoked this connection; connect again",
	"replaced":      "a newer connection of this client replaced this one; use its tokens",
	"refresh_reuse": "this connection was revoked because a refresh token was used twice; connect again",
	"code_reuse":    "this connection was revoked because its authorization code was used twice; connect again",
}

func (e *endedGrant) description() string {
	switch e.state {
	case GrantStateExpired:
		return "the refresh token expired after 30 days without use; connect again"
	case GrantStateAgedOut:
		return "the connection reached its 90 day limit; connect again"
	}
	if text, ok := revokedDescriptions[e.revokedReason]; ok {
		return text
	}
	return "this connection was revoked; connect again"
}

func (e *endedGrant) refusal() *oauthError {
	return &oauthError{http.StatusBadRequest, errInvalidGrant.code, e.description()}
}

func writeEndedGrant(w http.ResponseWriter, e *endedGrant) {
	body := map[string]string{"error": errInvalidGrant.code, "error_description": e.description(), "grant_state": e.state}
	if e.revokedReason != "" {
		body["revoked_reason"] = e.revokedReason
	}
	noStore(w)
	writeJSON(w, http.StatusBadRequest, body)
}

var (
	errRefreshAlreadyUsed = &oauthError{http.StatusBadRequest, errInvalidGrant.code, "this refresh token was already exchanged; use the refresh token that exchange returned"}
	errCodeExpired        = &oauthError{http.StatusBadRequest, errInvalidGrant.code, "the authorization code expired; start the connection again"}
	errCodeAlreadyUsed    = &oauthError{http.StatusBadRequest, errInvalidGrant.code, "the authorization code was already used; start the connection again"}
)
