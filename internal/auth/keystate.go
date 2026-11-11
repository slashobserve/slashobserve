package auth

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

const (
	keyExpiringWithin = 7 * 24 * time.Hour

	KeyStateRevoked  = "revoked"
	KeyStateExpired  = "expired"
	KeyStateExpiring = "expiring"
	KeyStateActive   = "active"
)

var KeyStates = lifecycle.Derived{Name: "service-account key", States: []lifecycle.DerivedState{
	{Name: KeyStateRevoked, When: "An admin revoked it."},
	{Name: KeyStateExpired, When: "Its expiry time has passed."},
	{Name: KeyStateExpiring, When: "It expires within 7 days."},
	{Name: KeyStateActive, When: "None of the above."},
}}

func keyState(revokedAt, expiresAt pgtype.Timestamptz, now time.Time) string {
	switch {
	case revokedAt.Valid:
		return KeyStateRevoked
	case !expiresAt.Valid:
		return KeyStateActive
	case !expiresAt.Time.After(now):
		return KeyStateExpired
	case expiresAt.Time.Sub(now) < keyExpiringWithin:
		return KeyStateExpiring
	}
	return KeyStateActive
}
