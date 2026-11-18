package loki_test

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestLokiTailsWithTheSharedLag(t *testing.T) {
	ds, _ := serve(t, 0, forward)
	lagged, ok := any(ds).(datasource.LagSource)
	if !ok {
		t.Fatal("Loki does not implement datasource.LagSource")
	}
	if got := lagged.TailLag(); got != 5*time.Second {
		t.Fatalf("TailLag = %v, want 5s", got)
	}
}
