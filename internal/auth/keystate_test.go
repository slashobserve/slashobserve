package auth

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestKeyStateOrdersRevokedExpiredExpiringActive(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	never := pgtype.Timestamptz{}
	week := 7 * 24 * time.Hour
	tests := []struct {
		name    string
		revoked pgtype.Timestamptz
		expires pgtype.Timestamptz
		want    string
	}{
		{"no expiry", never, never, "active"},
		{"far expiry", never, at(30 * 24 * time.Hour), "active"},
		{"exactly seven days left", never, at(week), "active"},
		{"seven days minus a second left", never, at(week - time.Second), "expiring"},
		{"a second left", never, at(time.Second), "expiring"},
		{"expires right now", never, at(0), "expired"},
		{"expired a day ago", never, at(-24 * time.Hour), "expired"},
		{"revoked with no expiry", at(-time.Hour), never, "revoked"},
		{"revoked beats expired", at(-time.Hour), at(-24 * time.Hour), "revoked"},
		{"revoked beats expiring", at(-time.Hour), at(time.Hour), "revoked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyState(tt.revoked, tt.expires, now); got != tt.want {
				t.Errorf("keyState = %s, want %s", got, tt.want)
			}
		})
	}
}
