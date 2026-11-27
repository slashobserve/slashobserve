package clickhouse

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestDriverNeverDialsALinkLocalOrMetadataAddress(t *testing.T) {
	for _, address := range []string{"169.254.169.254:9000", "[fe80::1]:9000", "0.0.0.0:9000"} {
		conn, err := dialPublic(t.Context(), address)
		if conn != nil {
			_ = conn.Close()
		}
		if !errors.Is(err, datasource.ErrLinkLocalAddress) {
			t.Errorf("dial %s = %v, want ErrLinkLocalAddress", address, err)
		}
	}
}
