package clickhouse

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestClickHouseTailsWithTheSharedLag(t *testing.T) {
	lagged, ok := any(linesDatasource(&fakeConn{})).(datasource.LagSource)
	if !ok {
		t.Fatal("ClickHouse does not implement datasource.LagSource")
	}
	if got := lagged.TailLag(); got != 5*time.Second {
		t.Fatalf("TailLag = %v, want 5s", got)
	}
}
