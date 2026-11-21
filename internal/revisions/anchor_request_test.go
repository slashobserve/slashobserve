package revisions_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestRequestOfCarriesTheStoredAnchorAndBlock(t *testing.T) {
	block := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	raw := []byte(`{"kind":"row","pos":{"x":0.4,"y":0.3},"row":{"service":"payments"},"shown":"440"}`)
	got := revisions.RequestOf(dbsqlc.AgentRequest{Anchor: raw, BlockID: block, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}})
	if got.Anchor == nil || got.Anchor.Describe() != "the row payments (440)" || got.BlockID != block.String() {
		t.Fatalf("anchor %+v, block %q", got.Anchor, got.BlockID)
	}
	plain := revisions.RequestOf(dbsqlc.AgentRequest{})
	if plain.Anchor != nil || plain.BlockID != "" {
		t.Fatalf("a request with none carried anchor %+v, block %q", plain.Anchor, plain.BlockID)
	}
	if broken := revisions.RequestOf(dbsqlc.AgentRequest{Anchor: []byte(`{"kind":`)}); broken.Anchor != nil {
		t.Fatalf("an unreadable anchor was carried: %+v", broken.Anchor)
	}
}
